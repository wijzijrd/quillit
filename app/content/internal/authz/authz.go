// Package authz answers the one question every project-scoped content
// endpoint needs answered before it acts: "does this user belong to this
// project?"
//
// content does not own that data — svc does (projects/project_members,
// app/svc/internal/handler/projects.go) — and #44 chose not to duplicate
// it here or embed it in the JWT (see the #44 PR description for the full
// reasoning: project membership is per-resource and changes on its own
// schedule via invite/join/remove, not something that fits a token that's
// awkward to invalidate early). Instead, SvcChecker asks svc directly over
// HTTP, on svc's internal-only membership-lookup route, with a short-lived
// cache so a brief svc outage degrades gracefully rather than making every
// content request fail.
package authz

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Checker answers whether userID belongs to projectID.
type Checker interface {
	IsMember(ctx context.Context, userID, projectID string) (bool, error)
}

// AllowAll is a Checker that treats every caller as a member of every
// project. It exists for tests that only care about exercising the
// "caller is authenticated" path (requireCaller) without also standing up
// real membership fixtures — production wiring (main.go) always uses
// SvcChecker.
type AllowAll struct{}

func (AllowAll) IsMember(context.Context, string, string) (bool, error) {
	return true, nil
}

// Static is a Checker backed by an in-memory membership table — for tests
// that need to exercise real accept/reject behavior (e.g. "reject a
// request from a user who isn't in this project's member list") without a
// live svc to call.
type Static struct {
	// Members maps userID -> set of projectIDs that user belongs to.
	Members map[string]map[string]bool
}

func (s Static) IsMember(_ context.Context, userID, projectID string) (bool, error) {
	return s.Members[userID][projectID], nil
}

// freshTTL bounds how long a membership answer is trusted before
// SvcChecker re-checks with svc. Mirrors svc's own revalidateTTL pattern
// (app/svc/internal/middleware/auth.go, used there to bound how long an
// "active account" answer is trusted) for the same underlying reason:
// membership changes on its own schedule (an editor can remove a member at
// any time via DELETE /api/projects/{id}/members/{userId}), so caching an
// answer forever would let a just-removed member keep acting on a project
// for as long as their session lasts.
const freshTTL = 30 * time.Second

// staleTolerance bounds how long a *stale* (older than freshTTL) cached
// answer stays usable as a fallback once svc stops answering. This is the
// mechanism that keeps a brief svc outage from making content wholesale
// unusable (the #44 acceptance criterion): only (user, project) pairs
// never resolved before during the outage window fail outright; anyone
// who made a request in roughly the last few minutes keeps working off
// their last known answer. Long enough to ride out a deploy or restart
// blip, short enough that someone removed from a project during an
// extended outage doesn't keep effective access indefinitely.
const staleTolerance = 5 * time.Minute

// httpTimeout bounds a single membership-check request to svc — long
// enough for a healthy same-network call, short enough that a hung svc
// doesn't hang every content request behind it.
const httpTimeout = 3 * time.Second

type cacheEntry struct {
	member    bool
	checkedAt time.Time
}

// SvcChecker is the production Checker: it asks svc's internal membership
// endpoint (GET {baseURL}/internal/projects/{projectID}/members/{userID})
// and caches the answer. That route is deliberately not under svc's /api
// prefix — app/ui/nginx.conf only forwards /api/ to svc, so it's
// unreachable from the browser; it exists purely for server-to-server
// callers on the same docker network content already depends on (see
// infra/docker-compose.yml: content has no exposed port at all, so it's
// already trusted with that same "reachable only inside the compose
// network" boundary).
type SvcChecker struct {
	baseURL string
	client  *http.Client

	mu    sync.Mutex
	cache map[string]cacheEntry
}

// NewSvcChecker builds a SvcChecker. httpClient may be nil, in which case
// a client with httpTimeout is constructed.
func NewSvcChecker(baseURL string, httpClient *http.Client) *SvcChecker {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: httpTimeout}
	}
	return &SvcChecker{
		baseURL: baseURL,
		client:  httpClient,
		cache:   make(map[string]cacheEntry),
	}
}

func cacheKey(userID, projectID string) string {
	return userID + "\x00" + projectID
}

func (c *SvcChecker) getCached(key string, maxAge time.Duration) (cacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.cache[key]
	if !ok || time.Since(e.checkedAt) > maxAge {
		return cacheEntry{}, false
	}
	return e, true
}

func (c *SvcChecker) setCached(key string, member bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Bound memory the same way svc's activityCache does: entries past
	// staleTolerance are never useful again (not even as a stale
	// fallback), so sweep them out on every write instead of growing
	// forever.
	for k, e := range c.cache {
		if time.Since(e.checkedAt) > staleTolerance {
			delete(c.cache, k)
		}
	}
	c.cache[key] = cacheEntry{member: member, checkedAt: time.Now()}
}

// IsMember answers whether userID belongs to projectID, per the caching
// and outage-tolerance rules documented on freshTTL/staleTolerance above.
func (c *SvcChecker) IsMember(ctx context.Context, userID, projectID string) (bool, error) {
	key := cacheKey(userID, projectID)

	if e, fresh := c.getCached(key, freshTTL); fresh {
		return e.member, nil
	}

	member, err := c.checkSvc(ctx, userID, projectID)
	if err == nil {
		c.setCached(key, member)
		return member, nil
	}

	// svc didn't give a definitive answer (network error, timeout, 5xx —
	// checkSvc only returns an error for these, never for a clean
	// "not a member" 404; see below). Fall back to a stale cached answer
	// rather than rejecting every request for every project outright.
	if e, ok := c.getCached(key, staleTolerance); ok {
		log.Printf("authz: svc membership check failed for %s (%v) — using cached answer from %s ago", key, err, time.Since(e.checkedAt).Round(time.Second))
		return e.member, nil
	}
	return false, fmt.Errorf("membership check unavailable: %w", err)
}

// checkSvc performs the actual HTTP round trip. A 200 means member; a 404
// means svc positively answered "no" (project doesn't exist, or userID
// isn't in it) — both are definitive, cacheable answers, not errors.
// Anything else (network failure, timeout, 5xx, unexpected status) is
// treated as "svc couldn't answer" and returned as an error for IsMember's
// stale-cache fallback to handle.
func (c *SvcChecker) checkSvc(ctx context.Context, userID, projectID string) (bool, error) {
	reqURL := fmt.Sprintf("%s/internal/projects/%s/members/%s", c.baseURL, url.PathEscape(projectID), url.PathEscape(userID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return false, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("GET %s: %w", reqURL, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("GET %s: unexpected status %d", reqURL, resp.StatusCode)
	}
}

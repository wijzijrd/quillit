package authz

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"

	v1 "github.com/quillit/gen/quillit/svc/v1"
	"github.com/quillit/gen/quillit/svc/v1/svcv1connect"
)

// stubSvcInternal is a minimal svcv1connect.SvcInternalServiceHandler
// backed by a plain func, so each test can supply exactly the
// success/CodeNotFound/other-error behavior it needs without a real svc.
type stubSvcInternal struct {
	fn func(ctx context.Context, req *connect.Request[v1.CheckMembershipRequest]) (*connect.Response[v1.CheckMembershipResponse], error)
}

func (s stubSvcInternal) CheckMembership(ctx context.Context, req *connect.Request[v1.CheckMembershipRequest]) (*connect.Response[v1.CheckMembershipResponse], error) {
	return s.fn(ctx, req)
}

// newTestRPCServer stands up a real SvcInternalService connect server
// (mirroring app/svc/internal/rpc/svc_internal.go's wiring, just backed by
// fn instead of a real DB) so SvcChecker is exercised against the actual
// connect wire protocol, not a hand-rolled stand-in.
func newTestRPCServer(t *testing.T, fn func(ctx context.Context, req *connect.Request[v1.CheckMembershipRequest]) (*connect.Response[v1.CheckMembershipResponse], error)) *httptest.Server {
	t.Helper()
	path, h := svcv1connect.NewSvcInternalServiceHandler(stubSvcInternal{fn})
	mux := http.NewServeMux()
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestSvcChecker_MemberReturnsTrue(t *testing.T) {
	var gotProjectID, gotUserID string
	srv := newTestRPCServer(t, func(_ context.Context, req *connect.Request[v1.CheckMembershipRequest]) (*connect.Response[v1.CheckMembershipResponse], error) {
		gotProjectID = req.Msg.GetProjectId()
		gotUserID = req.Msg.GetUserId()
		return connect.NewResponse(&v1.CheckMembershipResponse{IsMember: true, Role: "gm", ProjectType: "campaign"}), nil
	})
	c := NewSvcChecker(srv.URL, "test-secret", nil)

	ok, err := c.IsMember(t.Context(), "user-1", "proj-1")
	if err != nil {
		t.Fatalf("IsMember: %v", err)
	}
	if !ok {
		t.Error("IsMember = false, want true for a successful CheckMembership response")
	}
	if gotProjectID != "proj-1" || gotUserID != "user-1" {
		t.Errorf("request = (project=%q, user=%q), want (proj-1, user-1)", gotProjectID, gotUserID)
	}
}

func TestSvcChecker_NonMemberReturnsFalse(t *testing.T) {
	srv := newTestRPCServer(t, func(context.Context, *connect.Request[v1.CheckMembershipRequest]) (*connect.Response[v1.CheckMembershipResponse], error) {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("not a member"))
	})
	c := NewSvcChecker(srv.URL, "test-secret", nil)

	ok, err := c.IsMember(t.Context(), "user-1", "proj-1")
	if err != nil {
		t.Fatalf("IsMember: %v", err)
	}
	if ok {
		t.Error("IsMember = true, want false for a CodeNotFound response")
	}
}

func TestSvcChecker_CachesFreshAnswerWithoutRecontacting(t *testing.T) {
	var calls int32
	srv := newTestRPCServer(t, func(context.Context, *connect.Request[v1.CheckMembershipRequest]) (*connect.Response[v1.CheckMembershipResponse], error) {
		atomic.AddInt32(&calls, 1)
		return connect.NewResponse(&v1.CheckMembershipResponse{IsMember: true}), nil
	})
	c := NewSvcChecker(srv.URL, "test-secret", nil)

	for i := 0; i < 5; i++ {
		if _, err := c.IsMember(t.Context(), "user-1", "proj-1"); err != nil {
			t.Fatalf("IsMember: %v", err)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("svc was called %d times, want 1 (rest should be served from cache)", got)
	}
}

func TestSvcChecker_FallsBackToStaleCacheOnOutage(t *testing.T) {
	var svcUp atomic.Bool
	svcUp.Store(true)
	srv := newHijackingTestServer(t, &svcUp)
	c := NewSvcChecker(srv.URL, "test-secret", nil)
	// Force the cache to be "stale but within tolerance" by back-dating it
	// directly rather than sleeping freshTTL in a test.
	if _, err := c.IsMember(t.Context(), "user-1", "proj-1"); err != nil {
		t.Fatalf("initial IsMember: %v", err)
	}
	c.mu.Lock()
	e := c.cache[cacheKey("user-1", "proj-1")]
	e.checkedAt = time.Now().Add(-(freshTTL + time.Second))
	c.cache[cacheKey("user-1", "proj-1")] = e
	c.mu.Unlock()

	svcUp.Store(false)
	ok, err := c.IsMember(t.Context(), "user-1", "proj-1")
	if err != nil {
		t.Fatalf("IsMember during outage with stale cache available: %v", err)
	}
	if !ok {
		t.Error("IsMember = false, want true (stale cached answer) during outage")
	}
}

func TestSvcChecker_FailsClosedWithNoCacheDuringOutage(t *testing.T) {
	srv := newHijackingTestServer(t, nil)
	c := NewSvcChecker(srv.URL, "test-secret", nil)

	_, err := c.IsMember(t.Context(), "user-1", "proj-1")
	if err == nil {
		t.Fatal("IsMember returned no error for an unreachable svc with no cached answer, want an error (fail closed)")
	}
}

func TestSvcChecker_StaleCacheBeyondToleranceIsNotUsed(t *testing.T) {
	srv := newHijackingTestServer(t, nil)
	c := NewSvcChecker(srv.URL, "test-secret", nil)
	c.mu.Lock()
	c.cache[cacheKey("user-1", "proj-1")] = cacheEntry{member: true, checkedAt: time.Now().Add(-(staleTolerance + time.Second))}
	c.mu.Unlock()

	_, err := c.IsMember(t.Context(), "user-1", "proj-1")
	if err == nil {
		t.Fatal("IsMember returned no error for a cache entry older than staleTolerance, want an error (fail closed)")
	}
}

func TestSvcChecker_UnhealthyOtherPairsStillFailDuringOutage(t *testing.T) {
	// Ensures the outage-tolerance mechanism only rescues (user, project)
	// pairs that were already cached — it must not silently allow
	// everyone in just because svc is down for one pair.
	srv := newHijackingTestServer(t, nil)
	c := NewSvcChecker(srv.URL, "test-secret", nil)
	c.mu.Lock()
	c.cache[cacheKey("user-1", "proj-1")] = cacheEntry{member: true, checkedAt: time.Now()}
	c.mu.Unlock()

	if _, err := c.IsMember(t.Context(), "user-2", "proj-1"); err == nil {
		t.Error("IsMember for an uncached (user, project) pair succeeded during an outage, want an error")
	}
}

// newHijackingTestServer builds a server that, while "up" (svcUp reads
// true, or svcUp is nil meaning "always down"), answers CheckMembership
// through a real connect handler with IsMember: true; while "down", it
// hangs up the connection without a response — a connection-level failure,
// which is protocol-agnostic, so the connect client just surfaces it as a
// non-CodeNotFound error, exactly what checkSvc's fallback path is for.
func newHijackingTestServer(t *testing.T, svcUp *atomic.Bool) *httptest.Server {
	t.Helper()
	_, connectHandler := svcv1connect.NewSvcInternalServiceHandler(stubSvcInternal{
		fn: func(context.Context, *connect.Request[v1.CheckMembershipRequest]) (*connect.Response[v1.CheckMembershipResponse], error) {
			return connect.NewResponse(&v1.CheckMembershipResponse{IsMember: true}), nil
		},
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if svcUp != nil && svcUp.Load() {
			connectHandler.ServeHTTP(w, r)
			return
		}
		hj, ok := w.(http.Hijacker)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		conn, _, _ := hj.Hijack()
		conn.Close()
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAllowAll_AlwaysMember(t *testing.T) {
	ok, err := AllowAll{}.IsMember(t.Context(), "anyone", "any-project")
	if err != nil || !ok {
		t.Errorf("AllowAll.IsMember = (%v, %v), want (true, nil)", ok, err)
	}
}

func TestStatic_LooksUpMembership(t *testing.T) {
	s := Static{Members: map[string]map[string]bool{
		"user-1": {"proj-1": true},
	}}
	if ok, _ := s.IsMember(t.Context(), "user-1", "proj-1"); !ok {
		t.Error("expected user-1 to be a member of proj-1")
	}
	if ok, _ := s.IsMember(t.Context(), "user-1", "proj-2"); ok {
		t.Error("expected user-1 not to be a member of proj-2")
	}
	if ok, _ := s.IsMember(t.Context(), "user-2", "proj-1"); ok {
		t.Error("expected user-2 (unknown) not to be a member of proj-1")
	}
}

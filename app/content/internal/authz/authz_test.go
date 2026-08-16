package authz

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func TestSvcChecker_MemberReturns200(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/projects/proj-1/members/user-1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	})
	c := NewSvcChecker(srv.URL, nil)

	ok, err := c.IsMember(t.Context(), "user-1", "proj-1")
	if err != nil {
		t.Fatalf("IsMember: %v", err)
	}
	if !ok {
		t.Error("IsMember = false, want true for a 200 response")
	}
}

func TestSvcChecker_NonMemberReturns404(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	c := NewSvcChecker(srv.URL, nil)

	ok, err := c.IsMember(t.Context(), "user-1", "proj-1")
	if err != nil {
		t.Fatalf("IsMember: %v", err)
	}
	if ok {
		t.Error("IsMember = true, want false for a 404 response")
	}
}

func TestSvcChecker_CachesFreshAnswerWithoutRecontacting(t *testing.T) {
	var calls int32
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusOK)
	})
	c := NewSvcChecker(srv.URL, nil)

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
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !svcUp.Load() {
			// Simulate an outage: connection-level failure, not just a
			// non-2xx status, by hanging up without a response.
			hj, ok := w.(http.Hijacker)
			if !ok {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			conn, _, _ := hj.Hijack()
			conn.Close()
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	c := NewSvcChecker(srv.URL, nil)
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
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		conn, _, _ := hj.Hijack()
		conn.Close()
	})
	c := NewSvcChecker(srv.URL, nil)

	_, err := c.IsMember(t.Context(), "user-1", "proj-1")
	if err == nil {
		t.Fatal("IsMember returned no error for an unreachable svc with no cached answer, want an error (fail closed)")
	}
}

func TestSvcChecker_StaleCacheBeyondToleranceIsNotUsed(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		conn, _, _ := hj.Hijack()
		conn.Close()
	})
	c := NewSvcChecker(srv.URL, nil)
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
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		conn, _, _ := hj.Hijack()
		conn.Close()
	})
	c := NewSvcChecker(srv.URL, nil)
	c.mu.Lock()
	c.cache[cacheKey("user-1", "proj-1")] = cacheEntry{member: true, checkedAt: time.Now()}
	c.mu.Unlock()

	if _, err := c.IsMember(t.Context(), "user-2", "proj-1"); err == nil {
		t.Error("IsMember for an uncached (user, project) pair succeeded during an outage, want an error")
	}
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

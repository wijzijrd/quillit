package contentclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"

	v1 "github.com/quillit/gen/quillit/content/v1"
	"github.com/quillit/gen/quillit/content/v1/contentv1connect"
)

// stubContentInternal is a minimal contentv1connect.ContentInternalServiceHandler
// backed by plain func fields, so each test can supply exactly the
// behavior it needs without a real content service.
type stubContentInternal struct {
	getEntry             func(context.Context, *connect.Request[v1.GetEntryRequest]) (*connect.Response[v1.GetEntryResponse], error)
	notifyProjectDeleted func(context.Context, *connect.Request[v1.NotifyProjectDeletedRequest]) (*connect.Response[v1.NotifyProjectDeletedResponse], error)
}

func (s stubContentInternal) GetEntry(ctx context.Context, req *connect.Request[v1.GetEntryRequest]) (*connect.Response[v1.GetEntryResponse], error) {
	return s.getEntry(ctx, req)
}

func (s stubContentInternal) NotifyProjectDeleted(ctx context.Context, req *connect.Request[v1.NotifyProjectDeletedRequest]) (*connect.Response[v1.NotifyProjectDeletedResponse], error) {
	return s.notifyProjectDeleted(ctx, req)
}

func newTestContentServer(t *testing.T, stub stubContentInternal) *httptest.Server {
	t.Helper()
	path, h := contentv1connect.NewContentInternalServiceHandler(stub)
	mux := http.NewServeMux()
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestClient_Get_Success(t *testing.T) {
	var gotEntryID string
	srv := newTestContentServer(t, stubContentInternal{
		getEntry: func(_ context.Context, req *connect.Request[v1.GetEntryRequest]) (*connect.Response[v1.GetEntryResponse], error) {
			gotEntryID = req.Msg.GetEntryId()
			return connect.NewResponse(&v1.GetEntryResponse{
				Id: "e1", ProjectId: "proj-1", Title: "Mary", Body: "# Mary\n",
			}), nil
		},
	})

	c := NewClient(srv.URL, "test-secret", nil)
	e, err := c.Get(context.Background(), "e1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if e.ID != "e1" || e.ProjectID != "proj-1" || e.Title != "Mary" || e.Body != "# Mary\n" {
		t.Errorf("unexpected entry: %+v", e)
	}
	if gotEntryID != "e1" {
		t.Errorf("entryID sent = %q, want e1", gotEntryID)
	}
}

func TestClient_Get_ForwardsArbitraryEntryID(t *testing.T) {
	var gotEntryID string
	srv := newTestContentServer(t, stubContentInternal{
		getEntry: func(_ context.Context, req *connect.Request[v1.GetEntryRequest]) (*connect.Response[v1.GetEntryResponse], error) {
			gotEntryID = req.Msg.GetEntryId()
			return connect.NewResponse(&v1.GetEntryResponse{Id: req.Msg.GetEntryId()}), nil
		},
	})

	c := NewClient(srv.URL, "test-secret", nil)
	if _, err := c.Get(context.Background(), "weird/id x"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if gotEntryID != "weird/id x" {
		t.Errorf("entryID sent = %q, want %q", gotEntryID, "weird/id x")
	}
}

func TestClient_Get_NotFound(t *testing.T) {
	srv := newTestContentServer(t, stubContentInternal{
		getEntry: func(context.Context, *connect.Request[v1.GetEntryRequest]) (*connect.Response[v1.GetEntryResponse], error) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("entry not found"))
		},
	})

	c := NewClient(srv.URL, "test-secret", nil)
	_, err := c.Get(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestClient_Get_OtherErrorIsNotErrNotFound(t *testing.T) {
	srv := newTestContentServer(t, stubContentInternal{
		getEntry: func(context.Context, *connect.Request[v1.GetEntryRequest]) (*connect.Response[v1.GetEntryResponse], error) {
			return nil, connect.NewError(connect.CodeInternal, errors.New("db error"))
		},
	})

	c := NewClient(srv.URL, "test-secret", nil)
	_, err := c.Get(context.Background(), "e1")
	if err == nil {
		t.Fatal("expected an error for a CodeInternal response, got nil")
	}
	if errors.Is(err, ErrNotFound) {
		t.Error("a CodeInternal response should not be reported as ErrNotFound")
	}
}

func TestClient_NotifyProjectDeleted_Success(t *testing.T) {
	var gotProjectID string
	srv := newTestContentServer(t, stubContentInternal{
		notifyProjectDeleted: func(_ context.Context, req *connect.Request[v1.NotifyProjectDeletedRequest]) (*connect.Response[v1.NotifyProjectDeletedResponse], error) {
			gotProjectID = req.Msg.GetProjectId()
			return connect.NewResponse(&v1.NotifyProjectDeletedResponse{ProjectId: req.Msg.GetProjectId(), EntriesOrphaned: 3}), nil
		},
	})

	c := NewClient(srv.URL, "test-secret", nil)
	if err := c.NotifyProjectDeleted(context.Background(), "proj-1"); err != nil {
		t.Fatalf("NotifyProjectDeleted: %v", err)
	}
	if gotProjectID != "proj-1" {
		t.Errorf("projectID sent = %q, want proj-1", gotProjectID)
	}
}

func TestClient_NotifyProjectDeleted_ErrorIsError(t *testing.T) {
	srv := newTestContentServer(t, stubContentInternal{
		notifyProjectDeleted: func(context.Context, *connect.Request[v1.NotifyProjectDeletedRequest]) (*connect.Response[v1.NotifyProjectDeletedResponse], error) {
			return nil, connect.NewError(connect.CodeInternal, errors.New("db error"))
		},
	})

	c := NewClient(srv.URL, "test-secret", nil)
	if err := c.NotifyProjectDeleted(context.Background(), "proj-1"); err == nil {
		t.Error("expected an error for a CodeInternal response, got nil")
	}
}

func TestClient_NotifyProjectDeleted_UnreachableServerIsError(t *testing.T) {
	c := NewClient("http://127.0.0.1:1", "test-secret", nil) // nothing listens here
	if err := c.NotifyProjectDeleted(context.Background(), "proj-1"); err == nil {
		t.Error("expected an error for an unreachable content service, got nil")
	}
}

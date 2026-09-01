// Package rpc implements svc's server-to-server connect RPC surface:
// SvcInternalService (proto/quillit/svc/internal/v1, generated into
// github.com/quillit/gen/quillit/svc/v1). It is the connectrpc
// replacement for svc's old internal-only HTTP route
// GET /internal/projects/{id}/members/{userId}
// (formerly handler.ProjectsHandler.InternalMembership), called today by
// app/content/internal/authz.SvcChecker.checkSvc. That HTTP route is
// removed now that this RPC is its only caller.
package rpc

import (
	"context"
	"database/sql"
	"errors"

	"connectrpc.com/connect"

	v1 "github.com/quillit/gen/quillit/svc/v1"

	"github.com/quillit/svc/internal/db/sqlc"
	"github.com/quillit/svc/internal/handler"
)

// SvcInternalServer implements svcv1connect.SvcInternalServiceHandler.
type SvcInternalServer struct {
	q *sqlc.Queries
}

// NewSvcInternalServer builds a SvcInternalServer over db.
func NewSvcInternalServer(db *sql.DB) *SvcInternalServer {
	return &SvcInternalServer{q: sqlc.New(db)}
}

// CheckMembership answers whether userID belongs to projectID, backed by
// the exact same query (handler.MemberRole) the old HTTP handler used.
// Mirrors that handler's status-code branching mechanically:
//   - sql.ErrNoRows (project doesn't exist, or userID isn't in it — these
//     are deliberately not told apart) becomes connect.CodeNotFound, the
//     old route's 404.
//   - any other DB error becomes connect.CodeInternal, the old route's 500.
//   - success means userID is a member; IsMember is always true on a
//     non-error response, since sql.ErrNoRows above is what carries the
//     negative answer (see app/content/internal/authz.SvcChecker.checkSvc,
//     this RPC's only caller, for how CodeNotFound maps back to "not a
//     member" on the other side of the wire).
func (s *SvcInternalServer) CheckMembership(ctx context.Context, req *connect.Request[v1.CheckMembershipRequest]) (*connect.Response[v1.CheckMembershipResponse], error) {
	projectID := req.Msg.GetProjectId()
	userID := req.Msg.GetUserId()

	projectType, role, err := handler.MemberRole(ctx, s.q, projectID, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("not a member"))
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&v1.CheckMembershipResponse{
		IsMember:    true,
		Role:        role,
		ProjectType: projectType,
	}), nil
}

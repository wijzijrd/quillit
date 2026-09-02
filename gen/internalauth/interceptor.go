// Package internalauth implements the shared-secret authentication that
// gates every quillit-internal connect RPC — the connectrpc analogue of
// app/messaging/internal/handler/send.go's X-Messaging-Secret check,
// generalized to one header (X-Internal-Secret) shared across the three
// internal services introduced by Task 8's proto scaffolding
// (ContentInternalService, SvcInternalService, MessagingInternalService).
//
// Each of those services is both a caller and a callee of another (svc
// calls content and messaging; content calls svc; ...), so both a
// server-side (verifying) and a client-side (injecting) interceptor are
// provided here, built around the same secret string. They're deliberately
// separate constructors rather than one interceptor that branches on
// connect.Spec.IsClient: call sites read their intent directly —
// NewServerInterceptor goes into a handler's connect.WithInterceptors,
// NewClientInterceptor into a client's — and a service that is a client of
// one internal RPC and a server for another never has to hand the same
// interceptor value to both roles and trust a branch to keep them straight.
package internalauth

import (
	"context"
	"crypto/subtle"
	"errors"

	"connectrpc.com/connect"
)

// HeaderName is the header carrying the shared internal secret on every
// quillit-internal connect RPC.
const HeaderName = "X-Internal-Secret"

// errUnauthenticated is the error returned (wrapped in a
// connect.CodeUnauthenticated error) when the header is missing or wrong.
// Never wraps or echoes the caller-supplied value.
var errUnauthenticated = errors.New("invalid or missing " + HeaderName + " header")

// NewServerInterceptor returns a connect interceptor for a service handler
// that rejects any unary call whose X-Internal-Secret header is missing or
// doesn't match secret. Comparison uses crypto/subtle.ConstantTimeCompare,
// mirroring app/messaging/internal/handler/send.go's existing
// X-Messaging-Secret check exactly (including its length-check-before-compare
// shape), so a timing side channel can't leak how many leading bytes of a
// guessed secret were correct.
func NewServerInterceptor(secret string) connect.UnaryInterceptorFunc {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			got := req.Header().Get(HeaderName)
			if !secretsMatch(got, secret) {
				return nil, connect.NewError(connect.CodeUnauthenticated, errUnauthenticated)
			}
			return next(ctx, req)
		}
	})
}

// NewClientInterceptor returns a connect interceptor for a service client
// that stamps every outgoing unary request with the shared secret under
// X-Internal-Secret, so the receiving service's NewServerInterceptor
// accepts it.
func NewClientInterceptor(secret string) connect.UnaryInterceptorFunc {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			req.Header().Set(HeaderName, secret)
			return next(ctx, req)
		}
	})
}

// secretsMatch reports whether got equals want, in constant time relative
// to want's length. Mirrors send.go's Send handler check exactly: reject
// an empty header outright, require equal lengths before ever calling
// subtle.ConstantTimeCompare (it treats unequal-length inputs as an
// immediate, non-constant-time mismatch), then compare.
func secretsMatch(got, want string) bool {
	if got == "" || len(got) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

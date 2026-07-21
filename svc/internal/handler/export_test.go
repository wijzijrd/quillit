package handler

import (
	"context"

	"github.com/quillit/svc/internal/ws"
)

// HandleInboundForTest exposes the unexported chat frame dispatcher so external
// tests can drive share_card scoping without a real WebSocket connection.
func (h *ChatWSHandler) HandleInboundForTest(ctx context.Context, projectID, sessionID, senderID string, client *ws.Client, data []byte) {
	h.handleInbound(ctx, projectID, sessionID, senderID, client, data)
}

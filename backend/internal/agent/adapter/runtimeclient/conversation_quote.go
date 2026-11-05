package runtimeclient

import (
	"context"
	"encoding/json"
	"io"
	"net/url"

	"github.com/google/uuid"

	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// ReadConversationQuote reads and verifies an authorized public message for quoting.
func (c *Client) ReadConversationQuote(ctx context.Context, a identity.Principal, pid, sid uuid.UUID, ref app.QuoteRef) (app.ConversationQuote, error) {
	var out app.ConversationQuote
	query := url.Values{"actor_id": {a.ID.String()}, "project_id": {pid.String()}}
	response, err := c.Request(ctx, "getAgentRun", map[string]string{"rid": ref.RunID.String()}, query, nil, "")
	if err != nil {
		return out, err
	}
	raw, readErr := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	_ = response.Body.Close()
	var run RunSnapshot
	if readErr != nil || response.StatusCode != 200 || json.Unmarshal(raw, &run) != nil || run.SessionID != sid.String() {
		return out, domain.ErrNotFound
	}
	response, err = c.Request(ctx, "getAgentMessage", map[string]string{"rid": ref.RunID.String(), "messageId": ref.MessageID}, query, nil, "")
	if err != nil {
		return out, err
	}
	// Closing the read-only response cannot change the received result.
	defer func() { _ = response.Body.Close() }()
	raw, err = io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return out, err
	}
	var message PublicMessage
	if response.StatusCode != 200 || json.Unmarshal(raw, &message) != nil || message.RunID != ref.RunID.String() || message.MessageID != ref.MessageID || message.Kind != "assistant" || len(message.Content) > 65536 {
		return out, domain.ErrNotFound
	}
	return app.ConversationQuote{QuoteRef: ref, Text: message.Content, ContentHash: domain.Hash([]byte(message.Content))}, nil
}

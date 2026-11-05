package runtimeclient

import (
	"context"
	"encoding/json"
	"io"
	"time"

	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
)

// DeliverControls owns a bounded polling loop. Only committed application
// receipts acknowledge an outbox record; transport acceptance never does.
func (c *Client) DeliverControls(ctx context.Context, store app.BusinessStore, onError func(error)) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		for range 8 {
			decision, found, err := store.ClaimDecision(ctx, time.Now())
			if err != nil {
				if onError != nil {
					onError(err)
				}
				break
			}
			if !found {
				break
			}
			err = c.deliver(ctx, store, decision)
			if err != nil {
				if onError != nil {
					onError(err)
				}
				retryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
				retryErr := store.RetryDecision(retryCtx, decision.DecisionID, time.Now().Add(2*time.Second))
				cancel()
				if retryErr != nil && onError != nil {
					onError(retryErr)
				}
			}
		}
	}
}
func (c *Client) deliver(ctx context.Context, store app.BusinessStore, d domain.Decision) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	body := ControlRequest{ActorID: d.ActorID.String(), OrgID: d.OrgID.String(), ScopeID: d.ScopeID.String(), ProjectID: d.ProjectID.String(), CanvasID: canvasID(d), DecisionID: d.DecisionID.String(), RootRunID: d.RootRunID.String(), SessionID: d.SessionID.String(), AuthorizationRevision: d.AuthorizationRevision, Action: d.Action, ContentHash: d.ContentHash}
	response, err := c.Request(ctx, "applyAgentDecision", map[string]string{"rid": d.RootRunID.String()}, nil, body, "")
	if err != nil {
		return err
	}
	// Closing the read-only response cannot change the received result.
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != 200 {
		return domain.ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return domain.ErrUnavailable
	}
	var receipt domain.AppliedReceipt
	if json.Unmarshal(raw, &receipt) != nil || receipt.DecisionID != d.DecisionID || receipt.AuthorizationRevision != d.AuthorizationRevision {
		return domain.ErrConflict
	}
	return store.Acknowledge(ctx, d.RootRunID, receipt)
}
func canvasID(d domain.Decision) *string {
	if d.CanvasID == nil {
		return nil
	}
	s := d.CanvasID.String()
	return &s
}

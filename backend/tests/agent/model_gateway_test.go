package agent_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain/graph"
	"github.com/google/uuid"
)

func TestModelGatewayParameterValidation(t *testing.T) {
	ctx := context.Background()

	gw := application.NewDefaultModelGateway(
		func(ctx context.Context, req application.ModelRequest) (*application.ModelResponse, error) {
			return &application.ModelResponse{
				Content:      "response content",
				InputTokens:  100,
				OutputTokens: 50,
				CostMicros:   1500,
			}, nil
		},
		nil,
		45*time.Second,
	)

	t.Run("MissingModelReleaseID", func(t *testing.T) {
		req := application.ModelRequest{
			Model: domain.DialogueModel{
				ReleaseID:     uuid.Nil,
				ContextWindow: 8192,
				MaxTokens:     2048,
			},
		}
		_, err := gw.Generate(ctx, req)
		if !errors.Is(err, application.ErrModelNotFound) {
			t.Fatalf("expected ErrModelNotFound, got %v", err)
		}
	})

	t.Run("ContextWindowTooSmall", func(t *testing.T) {
		req := application.ModelRequest{
			Model: domain.DialogueModel{
				ReleaseID:     uuid.New(),
				ContextWindow: 512, // < 1024
				MaxTokens:     256,
			},
		}
		_, err := gw.Generate(ctx, req)
		if !errors.Is(err, application.ErrModelNotFound) {
			t.Fatalf("expected ErrModelNotFound, got %v", err)
		}
	})

	t.Run("MaxTokensExceedsContextWindow", func(t *testing.T) {
		req := application.ModelRequest{
			Model: domain.DialogueModel{
				ReleaseID:     uuid.New(),
				ContextWindow: 4096,
				MaxTokens:     4096, // >= ContextWindow
			},
		}
		_, err := gw.Generate(ctx, req)
		if !errors.Is(err, application.ErrMaxTokensInvalid) {
			t.Fatalf("expected ErrMaxTokensInvalid, got %v", err)
		}
	})

	t.Run("InputMessagesExceedContextWindow", func(t *testing.T) {
		hugeText := strings.Repeat("长剧本长文本超长内容", 3000) // ~24000 tokens
		req := application.ModelRequest{
			Model: domain.DialogueModel{
				ReleaseID:     uuid.New(),
				ContextWindow: 8192,
				MaxTokens:     2048,
			},
			Messages: []graph.Message{
				{Role: "user", Content: hugeText},
			},
		}
		_, err := gw.Generate(ctx, req)
		if !errors.Is(err, application.ErrContextExceeded) {
			t.Fatalf("expected ErrContextExceeded, got %v", err)
		}
	})

	t.Run("ValidRequestSucceeds", func(t *testing.T) {
		req := application.ModelRequest{
			Model: domain.DialogueModel{
				ReleaseID:     uuid.New(),
				ContextWindow: 8192,
				MaxTokens:     2048,
			},
			Messages: []graph.Message{
				{Role: "user", Content: "你好，请帮我分析剧本。"},
			},
		}
		resp, err := gw.Generate(ctx, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Content != "response content" {
			t.Errorf("unexpected content: %s", resp.Content)
		}
		if resp.CostMicros != 1500 {
			t.Errorf("unexpected cost micros: %d", resp.CostMicros)
		}
	})
}

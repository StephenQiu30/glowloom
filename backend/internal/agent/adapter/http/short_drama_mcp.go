package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
)

// ShortDramaToolHandler executes one domain-specific short-drama MCP tool within the caller's authorized scope.
type ShortDramaToolHandler func(ctx context.Context, scope domain.Scope, callID uuid.UUID, raw json.RawMessage) (domain.CallResult, error)

// ShortDramaExecutors maps tool names to concrete Go domain handlers backed by creation/media/execution services.
type ShortDramaExecutors map[string]ShortDramaToolHandler

type guardedWriteInput struct {
	ExpectedVersion  *int64 `json:"expected_version"`
	ExpectedRevision *int64 `json:"expectedRevision"`
}

type guardedBillableInput struct {
	DecisionID         string `json:"decision_id"`
	ManualCanvasRedraw bool   `json:"manual_canvas_redraw"`
}

func wrapShortDramaGuard(effect string, handler ShortDramaToolHandler) ShortDramaToolHandler {
	return func(ctx context.Context, scope domain.Scope, callID uuid.UUID, raw json.RawMessage) (domain.CallResult, error) {
		if effect == "write" {
			var guard guardedWriteInput
			if err := json.Unmarshal(raw, &guard); err != nil {
				return domain.CallResult{Status: domain.CallStatusFailed}, errors.New("optimistic_lock_required: expected_version must be >= 0")
			}
			version := guard.ExpectedVersion
			if version == nil {
				version = guard.ExpectedRevision
			}
			if version == nil || *version < 0 {
				return domain.CallResult{Status: domain.CallStatusFailed}, errors.New("optimistic_lock_required: expected_version must be >= 0")
			}
		}
		if effect == "billable" {
			var guard guardedBillableInput
			if err := json.Unmarshal(raw, &guard); err != nil || (guard.DecisionID == "" && !guard.ManualCanvasRedraw) {
				return domain.CallResult{Status: domain.CallStatusFailed}, errors.New("approval_required: billable tool requires decision_id or manual_canvas_redraw=true")
			}
		}
		if handler == nil {
			return domain.CallResult{Status: domain.CallStatusFailed}, fmt.Errorf("short-drama tool handler not wired")
		}
		return handler(ctx, scope, callID, raw)
	}
}

// ShortDramaMCPTools constructs the canonical 5-group AI short-drama MCP tool catalog for /internal/agent-mcp.
func ShortDramaMCPTools(executors ShortDramaExecutors) []app.Tool {
	specs := []struct {
		name        string
		description string
		effect      string
		roles       []string
		schema      string
	}{
		// Group 1: Script & Event Graph
		{
			name:        "script_get_event_graph",
			description: "Read structured chapter event graph for multi-episode continuity",
			effect:      "read",
			roles:       []string{"producer", "scriptwriter", "continuity_reviewer"},
			schema:      `{"type":"object","properties":{"episode_from":{"type":"integer"},"episode_to":{"type":"integer"}}}`,
		},
		{
			name:        "script_upsert_event_graph",
			description: "Upsert chapter event graph nodes with optimistic concurrency lock",
			effect:      "write",
			roles:       []string{"scriptwriter"},
			schema:      `{"type":"object","required":["expected_version","events"],"properties":{"expected_version":{"type":"integer"},"events":{"type":"array"}}}`,
		},
		{
			name:        "script_get_episode",
			description: "Read structured episode script",
			effect:      "read",
			roles:       []string{"producer", "scriptwriter", "production", "continuity_reviewer"},
			schema:      `{"type":"object","required":["episode_number"],"properties":{"episode_number":{"type":"integer"}}}`,
		},
		{
			name:        "script_save_episode",
			description: "Save structured 9:16 vertical short-drama episode script with optimistic lock",
			effect:      "write",
			roles:       []string{"scriptwriter"},
			schema:      `{"type":"object","required":["expected_version","episode_number","scenes"],"properties":{"expected_version":{"type":"integer"},"episode_number":{"type":"integer"},"scenes":{"type":"array"}}}`,
		},
		// Group 2: Bible & Subject Consistency
		{
			name:        "bible_list_assets",
			description: "List project bible characters, costumes, scenes, and props",
			effect:      "read",
			roles:       []string{"producer", "scriptwriter", "asset_director", "production", "continuity_reviewer"},
			schema:      `{"type":"object","properties":{}}`,
		},
		{
			name:        "subject_list",
			description: "List locked subject anchors (subject_id, turnaround sheets, voice_id)",
			effect:      "read",
			roles:       []string{"producer", "asset_director", "production", "continuity_reviewer"},
			schema:      `{"type":"object","properties":{}}`,
		},
		{
			name:        "subject_lock_reference",
			description: "Lock approved character turnaround or scene anchor into subject library",
			effect:      "write",
			roles:       []string{"asset_director"},
			schema:      `{"type":"object","required":["expected_version","name","asset_id"],"properties":{"expected_version":{"type":"integer"},"name":{"type":"string"},"asset_id":{"type":"string"}}}`,
		},
		{
			name:        "voice_bind_character",
			description: "Bind a persistent TTS voice_id to a locked subject",
			effect:      "write",
			roles:       []string{"asset_director"},
			schema:      `{"type":"object","required":["expected_version","subject_id","voice_id"],"properties":{"expected_version":{"type":"integer"},"subject_id":{"type":"string"},"voice_id":{"type":"string"}}}`,
		},
		// Group 3: Infinite Canvas
		{
			name:        "canvas_read_graph",
			description: "Read current infinite canvas node graph and selected nodes",
			effect:      "read",
			roles:       []string{"producer", "production", "continuity_reviewer"},
			schema:      `{"type":"object","properties":{"canvas_id":{"type":"string"}}}`,
		},
		{
			name:        "canvas_upsert_storyboard_nodes",
			description: "Create or update storyboard nodes and edges on infinite canvas",
			effect:      "write",
			roles:       []string{"production"},
			schema:      `{"type":"object","required":["expected_version","nodes"],"properties":{"expected_version":{"type":"integer"},"nodes":{"type":"array"}}}`,
		},
		{
			name:        "canvas_split_grid_2x2",
			description: "Slice a generated 2x2 grid storyboard image into 4 upscaled 9:16 canvas shot nodes",
			effect:      "write",
			roles:       []string{"production"},
			schema:      `{"type":"object","required":["expected_version","grid_asset_id","shot_ids"],"properties":{"expected_version":{"type":"integer"},"grid_asset_id":{"type":"string"},"shot_ids":{"type":"array"}}}`,
		},
		// Group 4: Multimodal Generation & Explicit Take Adoption
		{
			name:        "media_estimate_quote",
			description: "Estimate credit cost for batch image, 2x2 grid, I2V, TTS, or LipSync generation",
			effect:      "read",
			roles:       []string{"producer", "asset_director", "production"},
			schema:      `{"type":"object","required":["modality","count"],"properties":{"modality":{"type":"string"},"count":{"type":"integer"}}}`,
		},
		{
			name:        "media_submit_image",
			description: "Submit approved turnaround sheet, 2x2 grid, or single-shot redraw image generation",
			effect:      "billable",
			roles:       []string{"asset_director", "production"},
			schema:      `{"type":"object","required":["prompt"],"properties":{"prompt":{"type":"string"},"subject_ids":{"type":"array"},"decision_id":{"type":"string"},"manual_canvas_redraw":{"type":"boolean"}}}`,
		},
		{
			name:        "media_submit_video_i2v",
			description: "Submit approved first/last-frame image-to-video generation for a shot",
			effect:      "billable",
			roles:       []string{"production"},
			schema:      `{"type":"object","required":["shot_id","anchor_asset_id"],"properties":{"shot_id":{"type":"string"},"anchor_asset_id":{"type":"string"},"decision_id":{"type":"string"},"manual_canvas_redraw":{"type":"boolean"}}}`,
		},
		{
			name:        "media_submit_tts",
			description: "Submit approved character voice_id TTS dialogue audio",
			effect:      "billable",
			roles:       []string{"production"},
			schema:      `{"type":"object","required":["shot_id","voice_id","text"],"properties":{"shot_id":{"type":"string"},"voice_id":{"type":"string"},"text":{"type":"string"},"decision_id":{"type":"string"},"manual_canvas_redraw":{"type":"boolean"}}}`,
		},
		{
			name:        "media_submit_lipsync",
			description: "Submit approved selective LipSync for close-up dialogue shot",
			effect:      "billable",
			roles:       []string{"production"},
			schema:      `{"type":"object","required":["shot_id","video_asset_id","audio_asset_id"],"properties":{"shot_id":{"type":"string"},"video_asset_id":{"type":"string"},"audio_asset_id":{"type":"string"},"decision_id":{"type":"string"},"manual_canvas_redraw":{"type":"boolean"}}}`,
		},
		{
			name:        "media_get_task",
			description: "Poll status and output asset of an async media generation task",
			effect:      "read",
			roles:       []string{"producer", "asset_director", "production"},
			schema:      `{"type":"object","required":["task_id"],"properties":{"task_id":{"type":"string"}}}`,
		},
		{
			name:        "shot_adopt_take",
			description: "Explicitly adopt one generated take as the canonical shot for timeline assembly",
			effect:      "write",
			roles:       []string{"production"},
			schema:      `{"type":"object","required":["expected_version","shot_id","take_id"],"properties":{"expected_version":{"type":"integer"},"shot_id":{"type":"string"},"take_id":{"type":"string"}}}`,
		},
		// Group 5: Timeline & Export
		{
			name:        "timeline_assemble_adopted_shots",
			description: "Assemble explicitly adopted shot takes and audio tracks onto project timeline",
			effect:      "write",
			roles:       []string{"production"},
			schema:      `{"type":"object","required":["expected_version","episode_number"],"properties":{"expected_version":{"type":"integer"},"episode_number":{"type":"integer"}}}`,
		},
		{
			name:        "timeline_export_render",
			description: "Trigger FFmpeg final episode video render from assembled timeline",
			effect:      "write",
			roles:       []string{"production"},
			schema:      `{"type":"object","required":["expected_version","timeline_id"],"properties":{"expected_version":{"type":"integer"},"timeline_id":{"type":"string"}}}`,
		},
	}

	tools := make([]app.Tool, 0, len(specs))
	for _, spec := range specs {
		handler := executors[spec.name]
		tools = append(tools, app.Tool{
			Descriptor: app.ToolDescriptor{
				Name:            spec.name,
				Roles:           spec.roles,
				Description:     spec.description,
				Parameters:      json.RawMessage(spec.schema),
				Effect:          spec.effect,
				ContractVersion: "v2",
				Replay:          "idempotent",
			},
			Execute: wrapShortDramaGuard(spec.effect, handler),
		})
	}
	return tools
}

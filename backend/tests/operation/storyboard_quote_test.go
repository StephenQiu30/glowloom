package operation_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/domain"
)

func TestStoryboardQuoteSource(t *testing.T) {
	canvasID, boardID, shotID := uuid.New(), uuid.New(), uuid.New()
	valid := domain.CanvasSource{CanvasID: canvasID, NodeID: boardID, Revision: 6, ShotID: &shotID, ShotRevision: int64Pointer(2)}
	for _, tc := range []struct {
		name   string
		change func(*domain.CanvasSource)
	}{
		{"missing shot revision", func(source *domain.CanvasSource) { source.ShotRevision = nil }},
		{"zero shot revision", func(source *domain.CanvasSource) { source.ShotRevision = int64Pointer(0) }},
		{"missing shot ID", func(source *domain.CanvasSource) { source.ShotID = nil }},
		{"nil shot UUID", func(source *domain.CanvasSource) { source.ShotID = uuidPtr(uuid.Nil) }},
		{"batch row and shot", func(source *domain.CanvasSource) { source.RowID = uuidPtr(uuid.New()) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := valid
			tc.change(&source)
			if source.Valid() {
				t.Fatalf("accepted incomplete or conflicting shot identity: %+v", source)
			}
		})
	}
	raw, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip domain.CanvasSource
	if err := json.Unmarshal(raw, &roundTrip); err != nil || !roundTrip.Valid() ||
		*roundTrip.ShotID != shotID || *roundTrip.ShotRevision != 2 {
		t.Fatalf("storyboard source JSON = %s, %v", raw, err)
	}
	for _, original := range []domain.CanvasSource{
		{CanvasID: canvasID, NodeID: boardID, Revision: 6},
		{CanvasID: canvasID, NodeID: boardID, Revision: 6, RowID: &shotID},
	} {
		if !original.Valid() {
			t.Fatalf("rejected legacy node or batch row source: %+v", original)
		}
	}
}

func TestStoryboardQuoteInput(t *testing.T) {
	shotID := uuid.New()
	source := &domain.CanvasSource{CanvasID: uuid.New(), NodeID: uuid.New(), Revision: 3, ShotID: &shotID, ShotRevision: int64Pointer(1)}
	prompt := application.StoryboardShotPrompt(" 开场 ", "推门走入室内", " 好久不见 ", "", "雨声")
	if want := "镜头：开场\n动作：推门走入室内\n对白：好久不见\n声音：雨声"; prompt != want {
		t.Fatalf("storyboard prompt = %q, want %q", prompt, want)
	}
	valid := application.FreeQuoteItemInput{
		Capability: "video.generate", Mode: "text2video", Prompt: prompt,
		Params: json.RawMessage(`{"duration_ms":5000}`), OutputCount: 1, Source: source,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid storyboard quote: %v", err)
	}
	for _, tc := range []struct {
		name   string
		change func(*application.FreeQuoteItemInput)
	}{
		{"other capability", func(input *application.FreeQuoteItemInput) { input.Capability = "image.generate" }},
		{"other mode", func(input *application.FreeQuoteItemInput) { input.Mode = "image_to_video" }},
		{"multiple outputs", func(input *application.FreeQuoteItemInput) { input.OutputCount = 2 }},
		{"unbound media", func(input *application.FreeQuoteItemInput) {
			input.MediaInputs = []application.FreeQuoteMediaInput{{Role: "reference_image", MediaAssetID: uuid.New()}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := valid
			tc.change(&input)
			if err := input.Validate(); !errors.Is(err, application.ErrInvalidFreeQuote) {
				t.Fatalf("invalid storyboard quote: %v", err)
			}
		})
	}
}

package operation_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	promptapp "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/application"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

func TestGenerationEstimateUsesPublishedPriceWithoutCreatingSubmission(t *testing.T) {
	db := operationStoreDB(t)
	f := seedManagedFixture(t, db)
	actor, project := operationStoreProject(t, db)
	require.NoError(t, db.Exec(`UPDATE workspace.project SET allow_overseas_models=true WHERE id=?`, project).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.model_profile_version SET modes=ARRAY['text2image'], limits='{"max_outputs":4}' WHERE id=?`, f.identity.ModelProfileVersionID).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.price_rule_version SET unit='per_image', rule='{"base_micros":120000}' WHERE id=?`, f.identity.PriceRuleVersionID).Error)

	version, mode := f.identity.ModelProfileVersionID.String(), "text2image"
	draft := promptdomain.Draft{
		SchemaVersion: 1, ModelVersion: &version, Mode: &mode,
		Params:     map[string]any{"output_count": float64(2)},
		Document:   promptdomain.Document{Type: "doc", Content: []promptdomain.Paragraph{{Type: "paragraph"}}},
		References: []promptdomain.Reference{},
		LabelState: promptdomain.LabelState{
			HighWater:   map[string]int64{"image": 0, "video": 0, "audio": 0, "subject": 0},
			Allocations: map[string]promptdomain.LabelAllocation{},
		},
	}
	store := editorOperationStore(db)
	_, err := store.EstimateGeneration(t.Context(), actor, project, draft, false)
	require.ErrorIs(t, err, operationapp.ErrFreeQuoteModelMissing)

	seedFixtureProcurement(t, db, f.identity.PriceRuleVersionID, f.identity.ModelProfileVersionID)
	seedEditorReleaseAvailability(t, db, f)
	estimate, err := store.EstimateGeneration(t.Context(), actor, project, draft, false)
	require.NoError(t, err)
	require.Equal(t, operationapp.GenerationEstimate{Credits: "3", Kind: "estimate", Basis: "per_image", Quantity: "2"}, estimate)
	draft.Params["output_count"] = float64(1)
	one, err := store.EstimateGeneration(t.Context(), actor, project, draft, false)
	require.NoError(t, err)
	require.Equal(t, "2", one.Credits)

	var operations, submissions int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM operation.operation WHERE project_id=?`, project).Row().Scan(&operations))
	require.NoError(t, db.Raw(`SELECT count(*) FROM operation.prompt_submission WHERE project_id=?`, project).Row().Scan(&submissions))
	require.Zero(t, operations)
	require.Zero(t, submissions)

	draft.Document.Content[0].Content = []promptdomain.Inline{{Type: "text", Text: "生成一张图片"}}
	saved, err := editorDraftStore(db).SaveDraft(t.Context(), actor, promptapp.DraftWrite{ProjectID: project, Key: uuid.New(), Draft: draft})
	require.NoError(t, err)
	epoch := int64(0)
	quote, err := store.CreateFreeQuote(t.Context(), actor, operationapp.CreateFreeQuoteInput{
		ProjectID: project, RequestID: uuid.NewString(),
		FreeQuoteItemInput: operationapp.FreeQuoteItemInput{
			InputSource:             &promptdomain.InputSource{Kind: "prompt_draft", DraftID: saved.DraftID, Revision: saved.Revision},
			ExpectedSubmissionEpoch: &epoch,
		},
	})
	require.NoError(t, err)
	var detail operationapp.QuoteCostDetail
	require.NoError(t, json.Unmarshal(quote.QuoteDetail, &detail))
	require.Equal(t, one.Credits, detail.ExactCredits)
}

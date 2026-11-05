package operation_test

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"

	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/managed"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/staging"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

func TestManagedMediaRealStorageAndProbeAreIdempotent(t *testing.T) {
	objects := receiptTestObjects(t)
	db := operationStoreDB(t)
	for _, kind := range []string{"image", "video"} {
		t.Run(kind, func(t *testing.T) {
			f := seedManagedFixture(t, db)
			i := f.identity
			require.NoError(t, db.Exec(`UPDATE catalog.model_profile SET capability=? WHERE id=(SELECT model_profile_id FROM catalog.model_profile_version WHERE id=?)`, kind+".generate", i.ModelProfileVersionID).Error)
			require.NoError(t, db.Exec(`UPDATE catalog.model_profile_version SET moderation='provider' WHERE id=?`, i.ModelProfileVersionID).Error)
			require.NoError(t, db.Exec(`UPDATE operation.operation SET capability=? WHERE id=?`, kind+".generate", i.OperationID).Error)
			claim, err := f.store.ClaimProviderDispatch(t.Context(), i)
			require.NoError(t, err)
			require.True(t, claim.Claimed)
			var data []byte
			mime := "image/png"
			if kind == "image" {
				img := image.NewRGBA(image.Rect(0, 0, 64, 48))
				img.Set(0, 0, color.RGBA{R: 255, A: 255})
				var encoded bytes.Buffer
				require.NoError(t, png.Encode(&encoded, img))
				data = encoded.Bytes()
			} else {
				mime = "video/mp4"
				file := filepath.Join(t.TempDir(), "fixture.mp4")
				command := exec.CommandContext(t.Context(), "ffmpeg", "-v", "error", "-f", "lavfi", "-i", "color=c=blue:s=64x48:r=10", "-t", "1", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-y", file)
				output, err := command.CombinedOutput()
				require.NoError(t, err, string(output))
				data, err = os.ReadFile(file)
				require.NoError(t, err)
			}
			businessPinOperation(t, db, i)
			results := application.NewRoutedManagedResults(staging.NewRoutedFactory(businessStorageRegistry(t), pgmedia.NewStorageRoutes(db)), f.store)
			artifact, err := results.StageArtifact(t.Context(), i, 1, data, mime)
			require.NoError(t, err)
			require.NoError(t, results.Save(t.Context(), application.ManagedResult{Identity: i, State: "succeeded", Artifacts: []application.ManagedArtifact{artifact}, Usage: json.RawMessage(`{"billable_requests":1,"output_count":1}`)}))
			_, err = f.store.TransitionWorkflowOperation(t.Context(), application.TransitionInput{OperationID: i.OperationID, From: []domain.Status{domain.StatusSubmitting}, To: domain.StatusSucceeded})
			require.NoError(t, err)
			_, err = f.store.TransitionWorkflowOperation(t.Context(), application.TransitionInput{OperationID: i.OperationID, From: []domain.Status{domain.StatusSucceeded}, To: domain.StatusIngesting})
			require.NoError(t, err)
			businessPinOperation(t, db, i)
			activity := managed.NewRoutedIngestActivities(db, businessStorageRegistry(t), results, app.NewMediaWorkerScopeAccess, f.store)
			require.NoError(t, activity.Ingest(t.Context(), i.OperationID))
			require.NoError(t, activity.Ingest(t.Context(), i.OperationID))
			var rows []struct {
				Key, Kind, Status, ModerationStatus string
				Width, Height                       int
				DurationMS                          *int64
			}
			require.NoError(t, db.Raw(`SELECT a.object_key AS key,a.kind,a.status,a.moderation_status,a.width,a.height,a.duration_ms FROM operation.operation_output o JOIN media.media_asset a ON a.id=o.media_asset_id WHERE o.operation_id=? AND NOT o.is_delete`, i.OperationID).Scan(&rows).Error)
			require.Len(t, rows, 1)
			require.Equal(t, kind, rows[0].Kind)
			require.Equal(t, "ready", rows[0].Status)
			require.Equal(t, "passed", rows[0].ModerationStatus)
			require.Equal(t, 64, rows[0].Width)
			require.Equal(t, 48, rows[0].Height)
			if kind == "video" {
				require.NotNil(t, rows[0].DurationMS)
				require.Greater(t, *rows[0].DurationMS, int64(0))
			}
			exists, err := objects.Exists(t.Context(), rows[0].Key)
			require.NoError(t, err)
			require.True(t, exists)
		})
	}
}

func TestManagedReferenceSettlementRequiresFrozenQuoteCount(t *testing.T) {
	for _, tc := range []struct {
		name   string
		detail string
		valid  bool
	}{
		{"zero references", `{"reference_image_count":0}`, true},
		{"missing frozen count", `{}`, false},
		{"input count changed", `{"reference_image_count":2}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			database := operationStoreDB(t)
			f := seedManagedFixture(t, database)
			i := f.identity
			require.NoError(t, database.Exec(`UPDATE catalog.model_profile SET capability='image.generate' WHERE id=(SELECT model_profile_id FROM catalog.model_profile_version WHERE id=?)`, i.ModelProfileVersionID).Error)
			require.NoError(t, database.Exec(`UPDATE catalog.model_profile_version SET modes=ARRAY['text2image'],supports_query=false,supports_cancel=false,queue='provider',moderation='provider' WHERE id=?`, i.ModelProfileVersionID).Error)
			require.NoError(t, database.Exec(`UPDATE catalog.price_rule_version SET unit='per_image',rule='{"base_micros":11,"by_resolution":{"2K":2},"free_reference_images":1,"reference_image_micros":3,"reference_scope":"per_task"}' WHERE id=?`, i.PriceRuleVersionID).Error)
			require.NoError(t, database.Exec(`UPDATE operation.operation SET capability='image.generate',mode='text2image',params='{"size":"2K"}',quote_detail=?::jsonb WHERE id=?`, tc.detail, i.OperationID).Error)
			claim, err := f.store.ClaimProviderDispatch(t.Context(), i)
			require.NoError(t, err)
			require.True(t, claim.Claimed)
			results := application.NewManagedResults(&stageTestObjects{entries: map[string]stageTestObject{}}, f.store)
			var encoded bytes.Buffer
			require.NoError(t, png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 64, 48))))
			artifact, err := results.StageArtifact(t.Context(), i, 1, encoded.Bytes(), "image/png")
			require.NoError(t, err)
			require.NoError(t, results.Save(t.Context(), application.ManagedResult{Identity: i, State: "succeeded", Artifacts: []application.ManagedArtifact{artifact}, Usage: json.RawMessage(`{"output_count":1,"billable_requests":1}`)}))
			err = f.store.CompleteProviderCall(t.Context(), application.CompleteProviderCallInput{OperationID: i.OperationID, Action: "submit", Attempt: 1, Outcome: "ok", State: "completed", Managed: true})
			if !tc.valid {
				require.ErrorIs(t, err, application.ErrProviderCostUnknown)
				return
			}
			require.NoError(t, err)
			cost, err := f.store.LoadProviderCost(t.Context(), i.OperationID)
			require.NoError(t, err)
			require.Equal(t, int64(22), cost.ActualCostMicros)
		})
	}
}

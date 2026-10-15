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
			results := application.NewManagedResults(staging.NewObjects(objects), f.store)
			artifact, err := results.StageArtifact(t.Context(), i, 1, data, mime)
			require.NoError(t, err)
			require.NoError(t, results.Save(t.Context(), application.ManagedResult{Identity: i, State: "succeeded", Artifacts: []application.ManagedArtifact{artifact}, Usage: json.RawMessage(`{"billable_requests":1,"output_count":1}`)}))
			_, err = f.store.TransitionWorkflowOperation(t.Context(), application.TransitionInput{OperationID: i.OperationID, From: []domain.Status{domain.StatusSubmitting}, To: domain.StatusSucceeded})
			require.NoError(t, err)
			_, err = f.store.TransitionWorkflowOperation(t.Context(), application.TransitionInput{OperationID: i.OperationID, From: []domain.Status{domain.StatusSucceeded}, To: domain.StatusIngesting})
			require.NoError(t, err)
			activity := managed.NewScopedIngestActivities(db, objects, results, app.NewMediaWorkerScopeAccess, f.store)
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

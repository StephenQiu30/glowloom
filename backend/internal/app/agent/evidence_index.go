package agent

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	evidencepg "github.com/StephenQiu30/glowloom-server/backend/creation/evidence/adapter/postgres"
	script "github.com/StephenQiu30/glowloom-server/backend/creation/script/application"
	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

// EvidenceIndexHandler extracts only the current adopted script's verified text.
func EvidenceIndexHandler(db *gorm.DB, objects script.PrivateObjects) taskruntime.Handler {
	sources := script.NewSourceService(sourceStore(db), objects, time.Now)
	return taskruntime.WithReference(15*time.Minute, func(ctx context.Context, job taskruntime.Job, ref evidencepg.ScriptIndexRequest) error {
		if job.BusinessID != ref.BusinessID() || ref.ProjectID == uuid.Nil || ref.OrgID == uuid.Nil ||
			ref.ActorID == uuid.Nil || ref.VersionID == uuid.Nil {
			return errors.New("invalid script evidence job")
		}
		pending, err := evidencepg.PendingScriptIndex(db.WithContext(ctx), ref.ProjectID)
		if err != nil || pending != ref.VersionID {
			return err
		}
		actor := script.Actor{ID: ref.ActorID, OrgID: ref.OrgID}
		var chunks []evidencepg.ScriptChunk
		err = sources.VisitAdoptedSourceText(ctx, actor, ref.ProjectID, ref.VersionID, func(source uuid.UUID, plain string) {
			chunks = append(chunks, evidencepg.ScriptChunks(ref, source, plain)...)
		})
		if errors.Is(err, script.ErrConflict) || errors.Is(err, script.ErrNotFound) {
			// The durable current-version check also runs before activation.
			return nil
		}
		if err != nil {
			return err
		}
		return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			_, err := scopewire.ProvideWorkerProjectContentAccess(tx).Authorize(ctx, identity.Principal{ID: ref.ActorID, OrgID: ref.OrgID}, ref.ProjectID, false)
			if errors.Is(err, workspacesource.ErrProjectNotFound) || errors.Is(err, workspacesource.ErrScopeNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			return evidencepg.PublishScriptIndex(tx, ref, chunks)
		})
	})
}

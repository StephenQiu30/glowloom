package task

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	opdomain "github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

type mediaIngest interface {
	Ingest(context.Context, mediaapp.IngestInput) (mediaapp.IngestOutput, error)
	RecordModeration(context.Context, mediaapp.RecordModerationInput) (mediaapp.RecordModerationOutput, error)
}
type managedIngest interface {
	Ingest(context.Context, uuid.UUID) error
	IngestOriginal(context.Context, app.WorkflowControlDelivery) error
}
type mockParams struct {
	Outcome    ProviderSubmitOutcome        `json:"mock_outcome"`
	DelayMS    int64                        `json:"mock_delay_ms"`
	URLs       []string                     `json:"mock_result_urls"`
	Moderation mediadomain.ModerationStatus `json:"mock_moderation_status"`
}

// RegisterMedia registers handlers for admitted media review and ingest steps.
func RegisterMedia(h map[string]taskruntime.Handler, s OperationStore, m mediaIngest, p managedIngest) {
	h["operation.original"] = taskruntime.WithReference(20*time.Minute, func(ctx context.Context, j taskruntime.Job, d app.WorkflowControlDelivery) error {
		if p == nil {
			return mediaapp.ErrStorageNotReady
		}
		if d.EventID.String() != j.BusinessID || d.Action != "recover" {
			return app.ErrInvalidWorkflowControl
		}
		return p.IngestOriginal(ctx, d)
	})
	h["operation.ingest"] = operationHandler(s, func(ctx context.Context, l app.WorkflowOperation, _ app.TaskEvidence) (time.Time, error) {
		if l.Operation.Status != opdomain.StatusIngesting {
			return time.Time{}, ErrInvalidOperationInput
		}
		if l.Provider.AdapterKey != "mock" {
			if p == nil {
				return time.Time{}, mediaapp.ErrStorageNotReady
			}
			return time.Time{}, p.Ingest(ctx, l.Operation.ID)
		}
		var params mockParams
		if json.Unmarshal(l.Operation.Params, &params) != nil || len(params.URLs) == 0 || len(params.URLs) > int(l.Operation.OutputCount) || m == nil {
			return time.Time{}, ErrInvalidProviderResult
		}
		for idx, url := range params.URLs {
			var outputID string
			var reviewed bool
			for _, saved := range l.Outputs {
				if saved.SeqNo == int32(idx+1) {
					outputID = saved.ID.String()
					reviewed = saved.ModerationStatus == mediadomain.ModerationPassed || saved.ModerationStatus == mediadomain.ModerationRejected
					break
				}
			}
			if outputID == "" {
				output, err := m.Ingest(ctx, mediaapp.IngestInput{OperationID: l.Operation.ID.String(), SeqNo: idx + 1, URL: url})
				if err != nil {
					return time.Time{}, err
				}
				outputID = output.OutputID
			}
			if !reviewed {
				if _, err := m.RecordModeration(ctx, mediaapp.RecordModerationInput{OperationID: l.Operation.ID.String(), OutputID: outputID, Status: params.Moderation}); err != nil {
					return time.Time{}, err
				}
			}
		}
		return time.Time{}, nil
	}, nil)
}

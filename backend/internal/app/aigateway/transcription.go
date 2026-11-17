package aigateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pg "github.com/StephenQiu30/glowloom-server/backend/execution/gateway/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	media "github.com/StephenQiu30/glowloom-server/backend/media/application"
	tool "github.com/StephenQiu30/glowloom-server/backend/media/mediatool/application"
	transcript "github.com/StephenQiu30/glowloom-server/backend/media/mediatool/domain"
)

type trackedTranscriber struct {
	db     *gorm.DB
	native tool.Transcriber
}
type localProvider func(context.Context, application.Dispatch, func(json.RawMessage) error) (domain.Receipt, error)

func (f localProvider) Send(ctx context.Context, d application.Dispatch, emit func(json.RawMessage) error) (domain.Receipt, error) {
	return f(ctx, d, emit)
}

// TrackTranscriber routes configured local inference through the same zero-credit request and ledger lifecycle.
// Public inference remains disabled when no actual native service is configured.
func TrackTranscriber(db *gorm.DB, native tool.Transcriber) tool.Transcriber {
	if native == nil {
		return nil
	}
	return trackedTranscriber{db, native}
}
func (t trackedTranscriber) Transcribe(context.Context, *media.Downloaded, string, int64) (transcript.Transcript, error) {
	return transcript.Transcript{}, tool.ErrInvalidTranscription
}
func (t trackedTranscriber) TranscribeWork(ctx context.Context, id tool.TranscriptionWorkID, work tool.TranscriptionWork, file *media.Downloaded, language string, duration int64) (transcript.Transcript, error) {
	if file == nil || file.File == nil || len(file.SHA256) != 64 || id.ExecutionID == uuid.Nil || work.Job.ID != id.JobID {
		return transcript.Transcript{}, tool.ErrInvalidTranscription
	}
	actor := work.Actor
	var sid uuid.UUID
	if err := t.db.WithContext(ctx).Raw("SELECT owner_scope_id FROM workspace.project WHERE id=? AND NOT is_delete", work.Job.ProjectID).Row().Scan(&sid); err != nil {
		return transcript.Transcript{}, err
	}
	actor.ScopeID = sid
	auth := func(ctx context.Context, tx *gorm.DB, a identity.Principal, scope, project uuid.UUID, admin bool) error {
		if admin || a.ID != actor.ID || scope != sid || project != work.Job.ProjectID {
			return domain.ErrForbidden
		}
		if _, err := scopewire.ProvideScopeAccess(tx).Resolve(ctx, a.ID, scope, true); err != nil {
			return err
		}
		if err := projectScope(tx, scope, project); err != nil {
			return err
		}
		var n int
		q := tx.Raw(`SELECT 1 FROM mediatool.transcription_job WHERE id=? AND actor_id=? AND project_id=? AND attempt=? AND active_worker=? AND inference_state='submitted' AND status IN ('running','cancel_requested') FOR SHARE`, id.JobID, a.ID, project, id.Attempt, id.ExecutionID).Scan(&n)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 || n != 1 {
			return domain.ErrForbidden
		}
		return nil
	}
	store := pg.New(t.db, actor, auth)
	model, err := store.Model(ctx, "local/whisper", "local")
	if err != nil {
		return transcript.Transcript{}, err
	}
	raw, _ := json.Marshal(application.LocalInput{Model: model.Name, JobID: id.JobID, Attempt: id.Attempt, SHA256: file.SHA256, Language: language, DurationMS: duration})
	call, err := application.Prepare(model, actor.ID, sid, work.Job.ProjectID, uuid.Nil, fmt.Sprintf("transcription/%s/%d", id.JobID, id.Attempt), "transcription", raw)
	if err != nil {
		return transcript.Transcript{}, err
	}
	call.Deadline = time.Now().Add(tool.TranscriptionInferenceTimeout)
	provider := localProvider(func(ctx context.Context, _ application.Dispatch, _ func(json.RawMessage) error) (domain.Receipt, error) {
		result, err := t.native.Transcribe(ctx, file, language, duration)
		if err != nil && !errors.Is(err, tool.ErrNoSpeech) {
			return domain.Receipt{}, domain.ErrUnknown
		}
		if err == nil && result.Validate() != nil {
			return domain.Receipt{}, domain.ErrUnknown
		}
		body, _ := json.Marshal(struct {
			Transcript transcript.Transcript `json:"transcript"`
			NoSpeech   bool                  `json:"no_speech"`
		}{result, errors.Is(err, tool.ErrNoSpeech)})
		return domain.Receipt{Body: body, UpstreamID: id.JobID.String()}, nil
	})
	record, err := application.New(store, provider).Execute(ctx, call, nil)
	if err != nil || record.State != "settled" || record.Receipt == nil {
		return transcript.Transcript{}, tool.ErrInferenceUncertain
	}
	var result struct {
		Transcript transcript.Transcript `json:"transcript"`
		NoSpeech   bool                  `json:"no_speech"`
	}
	if json.Unmarshal(record.Receipt.Body, &result) != nil {
		return transcript.Transcript{}, tool.ErrInvalidTranscription
	}
	if result.NoSpeech {
		return transcript.Transcript{}, tool.ErrNoSpeech
	}
	return result.Transcript, nil
}

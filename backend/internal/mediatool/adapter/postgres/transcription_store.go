package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	mediadomain "github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
)

// TranscriptionSourceFactory binds the formal media node reader to the command transaction.
type TranscriptionSourceFactory func(*gorm.DB) application.TranscriptionSourceReader

// TranscriptionStore persists speech facts and consumes owning application ports.
type TranscriptionStore struct {
	db     *gorm.DB
	source TranscriptionSourceFactory
	media  MediaFactory
}

// NewTranscriptionStore injects existing private media and saved canvas ownership.
func NewTranscriptionStore(db *gorm.DB, source TranscriptionSourceFactory, media MediaFactory) *TranscriptionStore {
	return &TranscriptionStore{db: db, source: source, media: media}
}

type transcriptionRow struct {
	ID, ProjectID, OrgID, ActorID, CanvasID, NodeID    uuid.UUID
	ActorRole, Language, Status, Stage, InferenceState string
	SourceRevision, Revision                           int64
	Frozen, Result                                     []byte
	Progress, Attempt                                  int
	ResultSHA256, FailureCode                          *string
	ActiveWorker                                       *uuid.UUID
	CreatedAt, UpdatedAt                               time.Time
}

func (r transcriptionRow) public() domain.TranscriptionJob {
	return domain.TranscriptionJob{ID: r.ID, ProjectID: r.ProjectID, Source: domain.TranscriptionSource{CanvasID: r.CanvasID, NodeID: r.NodeID, Revision: r.SourceRevision}, Language: r.Language, Status: domain.TranscriptionStatus(r.Status), Stage: r.Stage, Progress: r.Progress, Attempt: r.Attempt, Revision: r.Revision, ResultSHA256: r.ResultSHA256, FailureCode: r.FailureCode, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}
func (r transcriptionRow) actor() identityapp.Principal {
	return identityapp.Principal{ID: r.ActorID, OrgID: r.OrgID, Role: identitydomain.Role(r.ActorRole)}
}
func readTranscription(tx *gorm.DB, project, id uuid.UUID, lock bool) (transcriptionRow, error) {
	var row transcriptionRow
	query := `SELECT * FROM mediatool.transcription_job WHERE id=?`
	args := []any{id}
	if project != uuid.Nil {
		query += ` AND project_id=?`
		args = append(args, project)
	}
	if lock {
		query += ` FOR UPDATE`
	}
	result := tx.Raw(query, args...).Scan(&row)
	if result.Error != nil {
		return row, fmt.Errorf("read transcription: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return row, application.ErrNotFound
	}
	return row, nil
}
func (s *TranscriptionStore) authorize(ctx context.Context, tx *gorm.DB, actor identityapp.Principal, project uuid.UUID, write bool) error {
	if s.media == nil {
		return application.ErrUnavailable
	}
	return normalize(s.media(tx).AuthorizeProject(ctx, actor, project, write))
}
func transcriptionLock(tx *gorm.DB, actor, key uuid.UUID) error {
	if actor == uuid.Nil || key == uuid.Nil {
		return application.ErrInvalidTranscription
	}
	return tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, "media-transcription-command/"+actor.String()+"/"+key.String()).Error
}
func transcriptionReplay(tx *gorm.DB, actor, key uuid.UUID, hash string) (domain.TranscriptionJob, bool, error) {
	var receipt struct {
		RequestHash string
		Response    []byte
	}
	result := tx.Raw(`SELECT request_hash,response FROM mediatool.transcription_command WHERE actor_id=? AND request_id=?`, actor, key).Scan(&receipt)
	if result.Error != nil {
		return domain.TranscriptionJob{}, false, result.Error
	}
	if result.RowsAffected == 0 {
		return domain.TranscriptionJob{}, false, nil
	}
	if receipt.RequestHash != hash {
		return domain.TranscriptionJob{}, false, application.ErrConflict
	}
	var job domain.TranscriptionJob
	if err := json.Unmarshal(receipt.Response, &job); err != nil {
		return job, false, err
	}
	return job, true, nil
}
func transcriptionInput(asset mediadomain.MediaAsset) (domain.FrozenSource, error) {
	if !asset.CanReference() || (asset.Kind != mediadomain.KindAudio && asset.Kind != mediadomain.KindVideo) || asset.ContainsRealPerson || asset.ConsentRecordID != nil || asset.SHA256 == nil || len(*asset.SHA256) != 64 || asset.ByteSize < 1 || asset.ByteSize > 500<<20 || asset.DurationMS == nil || *asset.DurationMS < 1 || *asset.DurationMS > 86400000 {
		return domain.FrozenSource{}, application.ErrInvalidTranscription
	}
	return domain.FrozenSource{AssetID: asset.ID, Revision: asset.Revision, Kind: string(asset.Kind), ObjectKey: asset.ObjectKey, MIMEType: asset.MimeType, ByteSize: asset.ByteSize, SHA256: *asset.SHA256, DurationMS: asset.DurationMS, Width: asset.Width, Height: asset.Height}, nil
}
func transcriptionQuota(tx *gorm.DB, project uuid.UUID) error {
	if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, "media-transcription-project/"+project.String()).Error; err != nil {
		return err
	}
	var count int64
	if err := tx.Raw(`SELECT count(*) FROM mediatool.transcription_job WHERE project_id=? AND status IN ('queued','running','cancel_requested')`, project).Scan(&count).Error; err != nil {
		return err
	}
	if count >= 2 {
		return application.ErrConflict
	}
	return nil
}

// Create freezes one current reviewed original, permanent command and real outbox.
func (s *TranscriptionStore) Create(ctx context.Context, actor identityapp.Principal, project, key uuid.UUID, input application.TranscriptionCreateInput) (domain.TranscriptionJob, error) {
	var job domain.TranscriptionJob
	if s == nil || s.db == nil {
		return job, application.ErrUnavailable
	}
	if project == uuid.Nil || input.CanvasID == uuid.Nil || input.NodeID == uuid.Nil || input.Revision < 1 {
		return job, application.ErrInvalidTranscription
	}
	if input.Language == "" {
		input.Language = "auto"
	}
	if !domain.ValidTranscriptionLanguage(input.Language) {
		return job, application.ErrInvalidTranscription
	}
	hash, err := commandHash("transcription_create", project, uuid.Nil, input)
	if err != nil {
		return job, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, actor, project, true); err != nil {
			return err
		}
		if err := transcriptionLock(tx, actor.ID, key); err != nil {
			return err
		}
		previous, found, err := transcriptionReplay(tx, actor.ID, key, hash)
		if err != nil {
			return err
		}
		if found {
			job = previous
			return nil
		}
		if s.source == nil {
			return application.ErrUnavailable
		}
		if err := transcriptionQuota(tx, project); err != nil {
			return err
		}
		assetID, err := s.source(tx).FreezeTranscriptionSource(ctx, actor, project, input.CanvasID, input.NodeID, input.Revision)
		if err != nil {
			mapped := normalize(err)
			if errors.Is(mapped, application.ErrInvalidExport) {
				return application.ErrInvalidTranscription
			}
			return mapped
		}
		assets, err := s.media(tx).Sources(ctx, actor, project, []uuid.UUID{assetID})
		if err != nil {
			return normalize(err)
		}
		if len(assets) != 1 || assets[0].ID != assetID {
			return application.ErrInvalidTranscription
		}
		original, err := transcriptionInput(assets[0])
		if err != nil {
			return err
		}
		frozen, err := json.Marshal(domain.FrozenTranscription{Input: original, Language: input.Language})
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		job = domain.TranscriptionJob{ID: uuid.New(), ProjectID: project, Source: domain.TranscriptionSource{CanvasID: input.CanvasID, NodeID: input.NodeID, Revision: input.Revision}, Language: input.Language, Status: domain.TranscriptionQueued, Stage: "queued", Attempt: 1, Revision: 1, CreatedAt: now, UpdatedAt: now}
		if err := tx.Exec(`INSERT INTO mediatool.transcription_job(id,project_id,org_id,actor_id,actor_role,canvas_id,node_id,source_revision,language,frozen,status,stage,progress,attempt,revision,inference_state,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?::jsonb,'queued','queued',0,1,1,'none',?,?)`, job.ID, project, actor.OrgID, actor.ID, string(actor.Role), input.CanvasID, input.NodeID, input.Revision, input.Language, string(frozen), now, now).Error; err != nil {
			return err
		}
		return recordTranscriptionCommand(tx, actor, key, hash, "requested", job, "start")
	})
	return job, err
}

// Get restores only public facts within current project membership.
func (s *TranscriptionStore) Get(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID) (domain.TranscriptionJob, error) {
	var job domain.TranscriptionJob
	if s == nil || s.db == nil {
		return job, application.ErrUnavailable
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, actor, project, false); err != nil {
			return err
		}
		row, err := readTranscription(tx, project, id, false)
		if err == nil {
			job = row.public()
		}
		return err
	})
	return job, err
}

// List restores bounded speech facts without exposing recognized text or object keys.
func (s *TranscriptionStore) List(ctx context.Context, actor identityapp.Principal, input application.ListInput) ([]domain.TranscriptionJob, error) {
	if s == nil || s.db == nil {
		return nil, application.ErrUnavailable
	}
	if input.ProjectID == uuid.Nil || input.Limit < 1 || input.Limit > 201 {
		return nil, application.ErrInvalidTranscription
	}
	var rows []transcriptionRow
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, actor, input.ProjectID, false); err != nil {
			return err
		}
		query := `SELECT * FROM mediatool.transcription_job WHERE project_id=?`
		args := []any{input.ProjectID}
		for _, filter := range []struct {
			column string
			id     uuid.UUID
		}{{"canvas_id", input.CanvasID}, {"node_id", input.NodeID}, {"id", input.After}} {
			if filter.id != uuid.Nil {
				op := "="
				if filter.column == "id" {
					op = "<"
				}
				query += " AND " + filter.column + op + "?"
				args = append(args, filter.id)
			}
		}
		query += ` ORDER BY id DESC LIMIT ?`
		args = append(args, input.Limit)
		return tx.Raw(query, args...).Scan(&rows).Error
	})
	if err != nil {
		return nil, err
	}
	jobs := make([]domain.TranscriptionJob, 0, len(rows))
	for _, row := range rows {
		jobs = append(jobs, row.public())
	}
	return jobs, nil
}

// Result serves exact recognized cues only after a successful real terminal response.
func (s *TranscriptionStore) Result(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID) (application.TranscriptionResult, error) {
	var output application.TranscriptionResult
	if s == nil || s.db == nil {
		return output, application.ErrUnavailable
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, actor, project, false); err != nil {
			return err
		}
		row, err := readTranscription(tx, project, id, false)
		if err != nil {
			return err
		}
		if row.Status != "succeeded" || row.ResultSHA256 == nil || row.InferenceState != "terminal" {
			return application.ErrConflict
		}
		var frozen domain.FrozenTranscription
		if json.Unmarshal(row.Frozen, &frozen) != nil {
			return application.ErrInvalidTranscription
		}
		assets, err := s.media(tx).Sources(ctx, actor, project, []uuid.UUID{frozen.Input.AssetID})
		if err != nil || len(assets) != 1 {
			return application.ErrConflict
		}
		current, err := transcriptionInput(assets[0])
		if err != nil {
			return application.ErrConflict
		}
		expected, _ := json.Marshal(frozen.Input)
		actual, _ := json.Marshal(current)
		if string(expected) != string(actual) {
			return application.ErrConflict
		}
		output.SourceAssetID, output.SourceAssetRevision, output.SourceSHA256 = frozen.Input.AssetID, frozen.Input.Revision, frozen.Input.SHA256
		if err := json.Unmarshal(row.Result, &output.Draft); err != nil || output.Draft.Validate() != nil {
			return application.ErrInvalidTranscription
		}
		raw, err := json.Marshal(output.Draft)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(raw)
		if hex.EncodeToString(digest[:]) != *row.ResultSHA256 {
			return application.ErrConflict
		}
		output.JobID, output.Revision, output.SHA256 = row.ID, row.Revision, *row.ResultSHA256
		output.SRT, err = output.Draft.SRT()
		return err
	})
	return output, err
}
func updateTranscription(tx *gorm.DB, row transcriptionRow) error {
	return tx.Exec(`UPDATE mediatool.transcription_job SET status=?,stage=?,progress=?,attempt=?,revision=?,inference_state=?,result=?::jsonb,result_sha256=?,failure_code=?,active_worker=?,actor_id=?,actor_role=?,updated_at=? WHERE id=?`, row.Status, row.Stage, row.Progress, row.Attempt, row.Revision, row.InferenceState, nullableJSON(row.Result), row.ResultSHA256, row.FailureCode, row.ActiveWorker, row.ActorID, row.ActorRole, row.UpdatedAt, row.ID).Error
}
func nullableJSON(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	return string(raw)
}

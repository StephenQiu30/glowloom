package aigateway

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"

	subjectwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/subject"
	subject "github.com/StephenQiu30/glowloom-server/backend/internal/creation/subject/application"
	subjectdomain "github.com/StephenQiu30/glowloom-server/backend/internal/creation/subject/domain"
	pg "github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	platform "github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// VoiceJobs is a project-free consumer of the same immutable AI sale and ledger.
// Optional authority factories are intended for explicit isolated integration fixtures.
type VoiceJobs struct {
	Runtime   *Runtime
	Authorize pg.Authority
	Access    func(*gorm.DB) subject.VoiceAccess
}

// NewVoiceJobs composes the project-free Subject speech consumer.
func NewVoiceJobs(r *Runtime) *VoiceJobs { return &VoiceJobs{Runtime: r} }
func (v *VoiceJobs) auth() pg.Authority {
	if v.Authorize != nil {
		return v.Authorize
	}
	return Authority
}
func (v *VoiceJobs) access(tx *gorm.DB) subject.VoiceAccess {
	if v.Access != nil {
		return v.Access(tx)
	}
	return subjectwire.NewVoiceAccess(tx)
}
func (v *VoiceJobs) request(ctx context.Context, tx *gorm.DB, a identity.Principal, write bool) (subject.VoiceOwner, error) {
	if err := v.auth()(ctx, tx, a, a.ScopeID, uuid.Nil, false); err != nil {
		return subject.VoiceOwner{}, err
	}
	owner, err := v.access(tx).Request(ctx, a, write)
	if err != nil {
		return owner, err
	}
	if owner.ScopeID != a.ScopeID || owner.PhysicalScopeID != a.OrgID || owner.PersonalOrgID == uuid.Nil || owner.PersonalScopeID == uuid.Nil {
		return owner, subject.ErrVoiceJobNotFound
	}
	return owner, nil
}

type voiceIntentRow struct {
	RequestID, ScopeID, ActorID, CommandKey, DraftIntentID uuid.UUID
	InputHash                                              string
	Input, Owner                                           []byte
	CreatedAt                                              time.Time
}

func validGatewayVoiceInput(in subject.CreateVoiceJobInput) bool {
	return in.DraftIntentID != uuid.Nil && in.ReleaseID != uuid.Nil && in.Name != "" && strings.TrimSpace(in.Name) == in.Name && utf8.ValidString(in.Name) && subjectdomain.UTF16Length(in.Name) <= 20 && !strings.ContainsRune(in.Name, 0) && strings.TrimSpace(in.Text) != "" && utf8.ValidString(in.Text) && utf8.RuneCountInString(in.Text) <= 500 && !strings.ContainsRune(in.Text, 0) && in.VoiceKey != "" && len(in.VoiceKey) <= 128 && !strings.ContainsAny(in.VoiceKey, "\r\n\x00") && (in.SpeechRate == nil || *in.SpeechRate >= -50 && *in.SpeechRate <= 100)
}
func voiceModelOption(m domain.Model) (subject.VoiceModelOption, bool) {
	out := subject.VoiceModelOption{ReleaseID: m.ID, ModelVersionID: m.ID, ModelKey: m.Name, DisplayName: m.Name, Voices: []subject.VoiceOption{}}
	if m.Validate() != nil || m.Protocol != "media" || m.Capability != "audio.generate" || !slices.Contains(m.Modes, "text2audio") || m.OutputPolicy != "provider_success_v1" {
		return out, false
	}
	p, ok := m.Parameters["speaker"]
	if !ok || p.Type != "string" || len(p.Enum) == 0 {
		return out, false
	}
	for _, raw := range p.Enum {
		var key string
		if json.Unmarshal(raw, &key) != nil || key == "" || len(key) > 128 || strings.ContainsAny(key, "\x00\r\n") {
			return out, false
		}
		out.Voices = append(out.Voices, subject.VoiceOption{Key: key, Name: key})
	}
	if len(p.Default) > 0 {
		_ = json.Unmarshal(p.Default, &out.DefaultVoiceKey)
	}
	return out, true
}

// Options exposes only configured audio products with explicit speaker enums.
func (v *VoiceJobs) Options(ctx context.Context, a identity.Principal) (subject.VoiceOptions, error) {
	out := subject.VoiceOptions{CurrentActorID: a.ID, CurrentScopeID: a.ScopeID, Models: []subject.VoiceModelOption{}, Reason: "speech_release_unavailable"}
	err := v.Runtime.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := v.request(ctx, tx, a, false); err != nil {
			return err
		}
		ready, err := pgmedia.NewStorageRoutes(tx).ReadyForWork(ctx)
		if err != nil {
			return err
		}
		if !ready || v.Runtime.Objects == nil || v.Runtime.Provider == nil {
			out.Reason = "voice_storage_unavailable"
			return nil
		}
		var rows []struct{ Document []byte }
		if err := tx.Raw(`SELECT document FROM ai.model m WHERE enabled AND document->>'protocol'='media' AND document->>'capability'='audio.generate' AND EXISTS(SELECT 1 FROM ai.route r JOIN ai.credential k ON k.id=r.credential_id WHERE r.model_id=m.id AND r.enabled AND k.state='active') ORDER BY name,id`).Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			var m domain.Model
			if json.Unmarshal(row.Document, &m) != nil {
				return domain.ErrInvalid
			}
			if option, ok := voiceModelOption(m); ok {
				out.Models = append(out.Models, option)
			}
		}
		out.Available = len(out.Models) > 0
		if out.Available {
			out.Reason = ""
		}
		return nil
	})
	return out, err
}

// Create binds the exact permanent key and draft to one queue/hold/route transaction.
func (v *VoiceJobs) Create(ctx context.Context, a identity.Principal, key uuid.UUID, in subject.CreateVoiceJobInput) (subject.VoiceJob, error) {
	var out subject.VoiceJob
	if key == uuid.Nil || !validGatewayVoiceInput(in) {
		return out, subject.ErrInvalidVoiceJob
	}
	hash := domain.Digest(canonical(in))
	err := v.Runtime.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		owner, err := v.request(ctx, tx, a, true)
		if err != nil {
			return err
		}
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", a.ScopeID.String()+"/"+a.ID.String()+"/voice/"+key.String()).Error; err != nil {
			return err
		}
		var prior voiceIntentRow
		q := tx.Raw("SELECT * FROM ai.voice_intent WHERE scope_id=? AND actor_id=? AND command_key=?", a.ScopeID, a.ID, key).Scan(&prior)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected == 1 {
			if prior.InputHash != hash {
				return subject.ErrVoiceJobKeyConflict
			}
			out, err = v.readJob(ctx, tx, prior)
			return err
		}
		if v.Runtime.Provider == nil || v.Runtime.Objects == nil {
			return subject.ErrVoiceUnavailable
		}
		var model domain.Model
		var row struct{ Document []byte }
		q = tx.Raw("SELECT document FROM ai.model WHERE id=? AND enabled", in.ReleaseID).Scan(&row)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 || json.Unmarshal(row.Document, &model) != nil {
			return subject.ErrVoiceUnavailable
		}
		option, ok := voiceModelOption(model)
		if !ok {
			return subject.ErrVoiceUnavailable
		}
		voiceOK := false
		for _, voice := range option.Voices {
			voiceOK = voiceOK || voice.Key == in.VoiceKey
		}
		if !voiceOK {
			return subject.ErrInvalidVoiceJob
		}
		params := map[string]any{"speaker": in.VoiceKey}
		if in.SpeechRate != nil {
			params["speech_rate"] = *in.SpeechRate
		}
		raw := canonical(domain.MediaInput{Model: model.Name, Capability: "audio.generate", Mode: "text2audio", Prompt: in.Text, Params: params, OutputCount: 1})
		call, err := application.Prepare(model, a.ID, a.ScopeID, uuid.Nil, uuid.Nil, "subject-voice/"+a.ID.String()+"/"+key.String(), "subject.voice", raw)
		if err != nil {
			return err
		}
		store := pg.New(tx, a, v.auth())
		quote, err := store.Quote(ctx, call)
		if err != nil {
			return err
		}
		call = quote.Call
		intent, err := pgmedia.NewStorageRoutes(tx).PinStorageWork(ctx, media.StorageWorkOwner{Kind: media.StorageWorkKindSubjectVoice, OwnerID: a.ScopeID, WorkID: call.ID})
		if err != nil {
			return err
		}
		checkKey := "personal/" + owner.PersonalOrgID.String() + "/" + a.ID.String() + "/audio/" + time.Now().UTC().Format("2006/01") + "/" + outputAssetID(call.ID).String() + ".mp3"
		if intent.State != media.StorageWorkOpen {
			return subject.ErrVoiceUnavailable
		}
		if err := v.Runtime.Objects.CheckWrite(ctx, platform.Ref{ProfileID: intent.ProfileID, Bucket: intent.Bucket, Key: checkKey}); err != nil {
			return err
		}
		if err := tx.Exec("INSERT INTO ai.voice_intent(request_id,scope_id,actor_id,command_key,draft_intent_id,input_hash,input,owner) VALUES(?,?,?,?,?,?,?::jsonb,?::jsonb)", call.ID, a.ScopeID, a.ID, key, in.DraftIntentID, hash, string(canonical(in)), string(canonical(owner))).Error; err != nil {
			return err
		}
		call.Deadline = time.Now().Add(15 * time.Minute)
		if _, err := store.Queue(ctx, call); err != nil {
			return err
		}
		prior = voiceIntentRow{RequestID: call.ID, ScopeID: a.ScopeID, ActorID: a.ID, CommandKey: key, DraftIntentID: in.DraftIntentID, InputHash: hash, Input: canonical(in), Owner: canonical(owner)}
		if err := tx.Raw("SELECT created_at FROM ai.voice_intent WHERE request_id=?", call.ID).Row().Scan(&prior.CreatedAt); err != nil {
			return err
		}
		out, err = v.readJob(ctx, tx, prior)
		return err
	})
	return out, err
}
func (v *VoiceJobs) readJob(_ context.Context, tx *gorm.DB, row voiceIntentRow) (subject.VoiceJob, error) {
	var in subject.CreateVoiceJobInput
	if json.Unmarshal(row.Input, &in) != nil || row.InputHash != domain.Digest(canonical(in)) {
		return subject.VoiceJob{}, domain.ErrConflict
	}
	out := subject.VoiceJob{ID: row.RequestID, CurrentActorID: row.ActorID, CurrentScopeID: row.ScopeID, DraftIntentID: row.DraftIntentID, Revision: 1, Status: "queued", Name: in.Name, Text: in.Text, ReleaseID: in.ReleaseID, ModelVersionID: in.ReleaseID, VoiceKey: in.VoiceKey, CreatedAt: row.CreatedAt, UpdatedAt: row.CreatedAt}
	var record struct {
		State, AttemptState string
		UpdatedAt           time.Time
	}
	q := tx.Raw("SELECT r.state,r.updated_at,(SELECT state FROM ai.attempt a WHERE a.request_id=r.id ORDER BY number DESC LIMIT 1) AS attempt_state FROM ai.request r WHERE r.id=?", row.RequestID).Scan(&record)
	if q.Error != nil {
		return out, q.Error
	}
	if q.RowsAffected != 1 {
		return out, domain.ErrInvalid
	}
	out.UpdatedAt = record.UpdatedAt
	switch record.State {
	case "sending":
		if record.AttemptState != "ready" {
			out.Status = "dispatching"
		}
	case "received", "settled":
		out.Status = "ingesting"
	case "pending":
		out.Status = "dispatching"
	case "unknown":
		out.Status = "unknown"
	case "released":
		out.Status = "failed"
		out.ErrorCode = "not_sent"
	}
	var output struct {
		Summary []byte
		State   string
	}
	q = tx.Raw("SELECT summary,state FROM ai.output WHERE request_id=?", row.RequestID).Scan(&output)
	if q.Error != nil {
		return out, q.Error
	}
	if output.State == "ready" {
		var summary OutputSummary
		if json.Unmarshal(output.Summary, &summary) != nil || summary.RequestID != row.RequestID || summary.Voice == nil || summary.Voice.AssetID != summary.AssetID {
			return out, domain.ErrConflict
		}
		out.Status = "succeeded"
		out.Result = summary.Voice
		out.Revision = 2
	}
	return out, nil
}

// Get reads one private actor-and-scope voice intent and its actual Media result.
func (v *VoiceJobs) Get(ctx context.Context, a identity.Principal, id uuid.UUID) (subject.VoiceJob, error) {
	var out subject.VoiceJob
	err := v.Runtime.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := v.request(ctx, tx, a, false); err != nil {
			return err
		}
		var row voiceIntentRow
		q := tx.Raw("SELECT * FROM ai.voice_intent WHERE request_id=? AND scope_id=? AND actor_id=?", id, a.ScopeID, a.ID).Scan(&row)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 {
			return subject.ErrVoiceJobNotFound
		}
		var err error
		out, err = v.readJob(ctx, tx, row)
		return err
	})
	return out, err
}

// Recover resolves the exact permanent command key without admitting a new job.
func (v *VoiceJobs) Recover(ctx context.Context, a identity.Principal, key uuid.UUID) (subject.VoiceRecovery, error) {
	out := subject.VoiceRecovery{State: "unrecorded"}
	if key == uuid.Nil {
		return out, subject.ErrInvalidVoiceJob
	}
	err := v.Runtime.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := v.request(ctx, tx, a, false); err != nil {
			return err
		}
		var row voiceIntentRow
		q := tx.Raw("SELECT * FROM ai.voice_intent WHERE scope_id=? AND actor_id=? AND command_key=?", a.ScopeID, a.ID, key).Scan(&row)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected == 0 {
			return nil
		}
		job, err := v.readJob(ctx, tx, row)
		if err != nil {
			return err
		}
		out.State = "recorded"
		out.Job = &job
		return nil
	})
	return out, err
}

var _ subject.VoiceJobs = (*VoiceJobs)(nil)

package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/domain"
)

// VoiceFactories retains each actual owner in the same job transaction.
type VoiceFactories struct {
	Access  func(*gorm.DB) application.VoiceAccess
	Catalog func(*gorm.DB) application.VoiceCatalog
	Storage func(*gorm.DB) application.VoiceStorage
}

// VoiceStore owns permanent space/draft intent and the single paid dispatch fence.
type VoiceStore struct {
	db        *gorm.DB
	factories VoiceFactories
}

// NewVoiceStore starts no workflows, tests or supplier calls.
func NewVoiceStore(db *gorm.DB, factories VoiceFactories) *VoiceStore {
	return &VoiceStore{db: db, factories: factories}
}

type voiceJobRow struct {
	ID, ActorID, OwnerScopeID, PhysicalScopeID, PersonalScopeID, PersonalOrgID, DraftIntentID, IdemKey uuid.UUID
	RequestSHA256, Name, Text                                                                          string
	ReleaseID, ProviderID, ConfigVersionID, CredentialID, ModelVersionID, PriceVersionID               uuid.UUID
	ResourceID, VoiceKey                                                                               string
	SpeechRate                                                                                         int
	UpstreamCost                                                                                       []byte
	Moderation                                                                                         string
	EventID, AssetID                                                                                   uuid.UUID
	StorageIntentID                                                                                    *uuid.UUID
	Status                                                                                             string
	Revision                                                                                           int64
	DispatchStartedAt                                                                                  *time.Time
	ExecutionID                                                                                        *uuid.UUID
	ProviderLogID, ErrorCode                                                                           string
	AudioLocationID, ManifestLocationID                                                                *uuid.UUID
	AudioSHA256                                                                                        *string
	AudioByteSize                                                                                      *int64
	Usage, Result                                                                                      []byte
	CreatedAt, UpdatedAt                                                                               time.Time
}

func (r voiceJobRow) work() (application.VoiceWork, error) {
	job := application.VoiceJob{ID: r.ID, CurrentActorID: r.ActorID, CurrentScopeID: r.OwnerScopeID, DraftIntentID: r.DraftIntentID, Revision: r.Revision, Status: r.Status, Name: r.Name, Text: r.Text, ReleaseID: r.ReleaseID, ModelVersionID: r.ModelVersionID, VoiceKey: r.VoiceKey, CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(), ErrorCode: r.ErrorCode}
	if len(r.Result) > 0 {
		var result application.VoiceJobResult
		if json.Unmarshal(r.Result, &result) != nil || result.AssetID != r.AssetID || result.AssetRevision < 1 || result.Kind != "audio" || result.MIMEType != "audio/mpeg" || result.LibraryItemID != r.AssetID || result.LibrarySource != "subject" {
			return application.VoiceWork{}, application.ErrVoiceUnavailable
		}
		job.Result = &result
	}
	if (job.Status == "succeeded") != (job.Result != nil) {
		return application.VoiceWork{}, application.ErrVoiceUnavailable
	}
	work := application.VoiceWork{Job: job, Owner: application.VoiceOwner{ScopeID: r.OwnerScopeID, PhysicalScopeID: r.PhysicalScopeID, PersonalScopeID: r.PersonalScopeID, PersonalOrgID: r.PersonalOrgID}, ProviderID: r.ProviderID, ConfigVersionID: r.ConfigVersionID, CredentialID: r.CredentialID, PriceVersionID: r.PriceVersionID, AssetID: r.AssetID, EventID: r.EventID, ResourceID: r.ResourceID, Moderation: r.Moderation, RequestSHA256: r.RequestSHA256, SpeechRate: r.SpeechRate, UpstreamCost: r.UpstreamCost, DispatchStartedAt: r.DispatchStartedAt}
	if r.ExecutionID != nil {
		work.ExecutionID = *r.ExecutionID
	}
	if r.StorageIntentID != nil {
		work.Storage.ID = *r.StorageIntentID
	}
	if r.AudioSHA256 != nil && r.AudioByteSize != nil && r.AudioLocationID != nil {
		receipt := application.VoiceReceipt{SchemaVersion: 1, JobID: r.ID, ActorID: r.ActorID, OwnerScopeID: r.OwnerScopeID, PersonalOrgID: r.PersonalOrgID, AssetID: r.AssetID, ReleaseID: r.ReleaseID, ModelVersionID: r.ModelVersionID, ProviderRequestID: r.ID, RequestSHA256: r.RequestSHA256, ProviderLogID: r.ProviderLogID, AudioLocationID: *r.AudioLocationID, AudioSHA256: *r.AudioSHA256, AudioByteSize: *r.AudioByteSize, MIMEType: "audio/mpeg", TerminalCode: 20000000, Usage: r.Usage}
		receipt.AudioKey = application.VoiceAudioKey(work)
		work.Receipt = &receipt
	}
	return work, nil
}

func (s *VoiceStore) request(ctx context.Context, tx *gorm.DB, a identity.Principal, write bool) (application.VoiceOwner, error) {
	if s == nil || s.factories.Access == nil || s.factories.Access(tx) == nil {
		return application.VoiceOwner{}, application.ErrVoiceUnavailable
	}
	owner, err := s.factories.Access(tx).Request(ctx, a, write)
	if err != nil {
		return owner, err
	}
	if owner.ScopeID != a.ScopeID || owner.PhysicalScopeID != a.OrgID || owner.PersonalScopeID == uuid.Nil || owner.PersonalOrgID == uuid.Nil {
		return owner, application.ErrVoiceJobNotFound
	}
	return owner, nil
}

func readVoiceRow(tx *gorm.DB, id uuid.UUID, lock bool) (voiceJobRow, error) {
	var row voiceJobRow
	query := `SELECT * FROM subject.voice_job WHERE id=?`
	if lock {
		query += ` FOR UPDATE`
	}
	r := tx.Raw(query, id).Scan(&row)
	if r.Error != nil {
		return row, r.Error
	}
	if r.RowsAffected != 1 {
		return row, application.ErrVoiceJobNotFound
	}
	return row, nil
}

func validVoiceInput(in application.CreateVoiceJobInput) bool {
	return in.DraftIntentID != uuid.Nil && in.ReleaseID != uuid.Nil && in.Name != "" && strings.TrimSpace(in.Name) == in.Name && utf8.ValidString(in.Name) && domain.UTF16Length(in.Name) <= 20 && !strings.ContainsRune(in.Name, 0) && strings.TrimSpace(in.Text) != "" && utf8.ValidString(in.Text) && utf8.RuneCountInString(in.Text) <= 500 && !strings.ContainsRune(in.Text, 0) && in.VoiceKey != "" && len(in.VoiceKey) <= 128 && !strings.ContainsAny(in.VoiceKey, "\x00\r\n") && (in.SpeechRate == nil || *in.SpeechRate >= -50 && *in.SpeechRate <= 100)
}

// Options never turns saving a connection into validation or activation.
func (s *VoiceStore) Options(ctx context.Context, a identity.Principal) (application.VoiceOptions, error) {
	out := application.VoiceOptions{CurrentActorID: a.ID, CurrentScopeID: a.ScopeID, Models: []application.VoiceModelOption{}, Reason: "speech_release_unavailable"}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := s.request(ctx, tx, a, false); err != nil {
			return err
		}
		if s.factories.Storage == nil || s.factories.Storage(tx) == nil {
			return application.ErrVoiceUnavailable
		}
		ready, err := s.factories.Storage(tx).Ready(ctx)
		if err != nil {
			return err
		}
		if !ready {
			out.Reason = "voice_storage_unavailable"
			return nil
		}
		if s.factories.Catalog == nil || s.factories.Catalog(tx) == nil {
			return application.ErrVoiceUnavailable
		}
		releases, err := s.factories.Catalog(tx).List(ctx)
		if err != nil {
			return err
		}
		for _, release := range releases {
			out.Models = append(out.Models, release.VoiceModelOption)
		}
		out.Available = len(out.Models) > 0
		if out.Available {
			out.Reason = ""
		}
		return nil
	})
	return out, err
}

// Create atomically admits a permanent intent and a proven outbox delivery.
// An existing key can schedule recovery of the same job, never a second send.
func (s *VoiceStore) Create(ctx context.Context, a identity.Principal, key uuid.UUID, in application.CreateVoiceJobInput) (application.VoiceJob, error) {
	if key == uuid.Nil || !validVoiceInput(in) {
		return application.VoiceJob{}, application.ErrInvalidVoiceJob
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return application.VoiceJob{}, err
	}
	digest := sha256.Sum256(raw)
	hash := hex.EncodeToString(digest[:])
	var out application.VoiceJob
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		owner, err := s.request(ctx, tx, a, true)
		if err != nil {
			return err
		}
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, "subject-voice/"+a.ID.String()+"/"+key.String()).Error; err != nil {
			return err
		}
		var prior voiceJobRow
		r := tx.Raw(`SELECT * FROM subject.voice_job WHERE actor_id=? AND idem_key=? FOR UPDATE`, a.ID, key).Scan(&prior)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 1 {
			if prior.OwnerScopeID != owner.ScopeID || prior.PhysicalScopeID != owner.PhysicalScopeID || prior.RequestSHA256 != hash {
				return application.ErrVoiceJobKeyConflict
			}
			work, err := prior.work()
			if err != nil {
				return err
			}
			out = work.Job
			if out.Status != "succeeded" && out.Status != "failed" {
				return enqueueVoice(tx, work, uuid.New(), work.DispatchStartedAt != nil)
			}
			return nil
		}
		if s.factories.Catalog == nil || s.factories.Storage == nil {
			return application.ErrVoiceUnavailable
		}
		ready, err := s.factories.Storage(tx).Ready(ctx)
		if err != nil {
			return err
		}
		if !ready {
			return application.ErrVoiceUnavailable
		}
		release, err := s.factories.Catalog(tx).Select(ctx, in.ReleaseID, in.VoiceKey)
		if err != nil {
			return err
		}
		id, eventID, assetID := uuid.New(), uuid.New(), uuid.New()
		storage, err := s.factories.Storage(tx).Pin(ctx, owner.ScopeID, id)
		if err != nil {
			return err
		}
		if storage.ID == uuid.Nil || storage.State != "open" {
			return application.ErrVoiceUnavailable
		}
		rate := 0
		if in.SpeechRate != nil {
			rate = *in.SpeechRate
		}
		now := time.Now().UTC()
		r = tx.Exec(`INSERT INTO subject.voice_job(id,actor_id,owner_scope_id,physical_scope_id,personal_scope_id,personal_org_id,draft_intent_id,idem_key,request_sha256,name,text,release_id,provider_id,config_version_id,credential_id,model_version_id,price_version_id,resource_id,voice_key,speech_rate,upstream_cost,moderation,event_id,asset_id,storage_intent_id,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?::jsonb,?,?,?,?,?,?)`, id, a.ID, owner.ScopeID, owner.PhysicalScopeID, owner.PersonalScopeID, owner.PersonalOrgID, in.DraftIntentID, key, hash, in.Name, in.Text, release.ReleaseID, release.ProviderID, release.ConfigVersionID, release.CredentialID, release.ModelVersionID, release.PriceVersionID, release.ResourceID, in.VoiceKey, rate, string(release.UpstreamCost), release.Moderation, eventID, assetID, storage.ID, now, now)
		if r.Error != nil {
			return r.Error
		}
		row, err := readVoiceRow(tx, id, false)
		if err != nil {
			return err
		}
		work, err := row.work()
		if err != nil {
			return err
		}
		out = work.Job
		if err := auditVoice(tx, work, "created"); err != nil {
			return err
		}
		return enqueueVoice(tx, work, eventID, false)
	})
	return out, err
}

// Get binds every read to both its creator and the original target space.
func (s *VoiceStore) Get(ctx context.Context, a identity.Principal, id uuid.UUID) (application.VoiceJob, error) {
	var out application.VoiceJob
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := s.request(ctx, tx, a, false); err != nil {
			return err
		}
		row, err := readVoiceRow(tx, id, false)
		if err != nil {
			return err
		}
		if row.ActorID != a.ID || row.OwnerScopeID != a.ScopeID || row.PhysicalScopeID != a.OrgID {
			return application.ErrVoiceJobNotFound
		}
		work, err := row.work()
		out = work.Job
		return err
	})
	return out, err
}

// Recover never searches for the newest task in a space or another draft.
func (s *VoiceStore) Recover(ctx context.Context, a identity.Principal, key uuid.UUID) (application.VoiceRecovery, error) {
	out := application.VoiceRecovery{State: "unrecorded"}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := s.request(ctx, tx, a, false); err != nil {
			return err
		}
		var row voiceJobRow
		r := tx.Raw(`SELECT * FROM subject.voice_job WHERE actor_id=? AND idem_key=?`, a.ID, key).Scan(&row)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 0 {
			return nil
		}
		if row.OwnerScopeID != a.ScopeID || row.PhysicalScopeID != a.OrgID {
			return application.ErrVoiceJobKeyConflict
		}
		work, err := row.work()
		if err != nil {
			return err
		}
		out.State = "recorded"
		out.Job = &work.Job
		return nil
	})
	return out, err
}

func enqueueVoice(tx *gorm.DB, work application.VoiceWork, eventID uuid.UUID, recovery bool) error {
	delivery := application.VoiceDelivery{EventID: eventID, JobID: work.Job.ID, ActorID: work.Job.CurrentActorID, ScopeID: work.Owner.ScopeID, RecoveryOnly: recovery}
	body, err := json.Marshal(struct {
		EventID    uuid.UUID                 `json:"event_id"`
		EventType  string                    `json:"event_type"`
		OccurredAt time.Time                 `json:"occurred_at"`
		Data       application.VoiceDelivery `json:"data"`
	}{eventID, application.VoiceTopic, time.Now().UTC(), delivery})
	if err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,?,?,?::jsonb)`, eventID, application.VoiceTopic, work.Job.ID.String(), string(body)).Error
}

var _ application.VoiceJobs = (*VoiceStore)(nil)

package postgres

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	subject "github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
)

// SubjectVoiceCatalog implements the Subject consumer's frozen speech-release
// port. Its caller retains current actor/scope locks in the same transaction.
type SubjectVoiceCatalog struct{ db *gorm.DB }

// NewSubjectVoiceCatalog reads actual independent speech credentials and releases.
func NewSubjectVoiceCatalog(db *gorm.DB) *SubjectVoiceCatalog { return &SubjectVoiceCatalog{db: db} }

type subjectSpeechRow struct {
	ReleaseID, ModelVersionID, ProviderID, ConfigVersionID, CredentialID, PriceVersionID uuid.UUID
	ModelKey, DisplayName, ResourceID, Moderation                                        string
	Config, ParamSchema, UpstreamCost                                                    []byte
}

const subjectSpeechQuery = `SELECT r.id AS release_id,v.id AS model_version_id,p.id AS provider_id,config.id AS config_version_id,key.id AS credential_id,price.id AS price_version_id,m.model_key,m.display_name,v.provider_model_id AS resource_id,v.moderation,config.config,v.param_schema,r.upstream_cost
 FROM catalog.model_profile m JOIN catalog.provider p ON p.id=m.provider_id
 JOIN catalog.model_release r ON r.id=m.active_release_id AND r.model_id=m.id
 JOIN catalog.model_profile_version v ON v.id=r.model_version_id AND v.model_profile_id=m.id
 JOIN catalog.provider_config_version config ON config.id=r.config_version_id AND config.provider_id=p.id
 JOIN catalog.provider_preset_version pv ON pv.id=config.preset_version_id
 JOIN catalog.provider_preset preset ON preset.id=pv.preset_id
 JOIN catalog.price_rule_version price ON price.id=r.price_version_id AND price.model_profile_id=m.id
 JOIN LATERAL (SELECT c.id FROM catalog.provider_credential c WHERE c.provider_id=p.id AND c.status='active' AND NOT c.is_delete AND NOT EXISTS(SELECT 1 FROM unnest(v.modes) required(mode) WHERE NOT EXISTS(SELECT 1 FROM catalog.model_validation validation JOIN operation.operation op ON op.id=validation.operation_id AND op.status='completed' AND NOT op.is_delete JOIN operation.provider_binding execution ON execution.operation_id=op.id AND execution.release_id=r.id JOIN operation.provider_credential_binding used_key ON used_key.operation_id=op.id AND used_key.credential_id=c.id WHERE validation.release_id=r.id AND validation.credential_id=c.id AND validation.mode=required.mode AND validation.status='succeeded' AND validation.adapter_version='ai-provider-v1' AND validation.evidence->>'kind'='real_provider' AND op.mode=validation.mode AND op.model_profile_version_id=r.model_version_id AND op.price_rule_version_id=r.price_version_id)) ORDER BY c.create_time DESC,c.id DESC LIMIT 1) chosen ON true
 JOIN catalog.provider_credential key ON key.id=chosen.id
 WHERE p.adapter_key='volcengine_speech' AND p.status='active' AND NOT p.is_delete AND m.status='active' AND NOT m.is_delete
 AND r.status='active' AND NOT v.is_delete AND v.provider_model_id='seed-tts-2.0' AND v.moderation='provider' AND v.queue='provider'
 AND NOT preset.emergency_disabled AND NOT preset.archived AND NOT price.is_delete AND price.effective_from<=now()
 AND (m.capability='audio.generate' AND v.modes=ARRAY['text2audio']::text[] OR m.capability='audio.tts' AND v.modes=ARRAY['tts']::text[])`

func speechOptions(raw []byte) ([]subject.VoiceOption, string, error) {
	var fields []struct {
		Field, Component, Type string
		Default                json.RawMessage
		Enum                   []json.RawMessage
	}
	if json.Unmarshal(raw, &fields) != nil {
		return nil, "", subject.ErrVoiceUnavailable
	}
	options := []subject.VoiceOption{}
	defaultKey := ""
	for _, field := range fields {
		if field.Field != "voice_id" && field.Field != "speaker" {
			continue
		}
		if field.Type != "string" || len(options) > 0 {
			return nil, "", subject.ErrVoiceUnavailable
		}
		_ = json.Unmarshal(field.Default, &defaultKey)
		for _, entry := range field.Enum {
			var key string
			if json.Unmarshal(entry, &key) != nil || !speechKey(key) {
				return nil, "", subject.ErrVoiceUnavailable
			}
			options = append(options, subject.VoiceOption{Key: key, Name: key})
		}
		if len(options) == 0 && speechKey(defaultKey) {
			options = append(options, subject.VoiceOption{Key: defaultKey, Name: defaultKey})
		}
	}
	if len(options) == 0 {
		return nil, "", subject.ErrVoiceUnavailable
	}
	if !slices.ContainsFunc(options, func(o subject.VoiceOption) bool { return o.Key == defaultKey }) {
		defaultKey = ""
	}
	return options, defaultKey, nil
}

func speechKey(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, c := range value {
		if c != '_' && c != '-' && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

func (r subjectSpeechRow) release() (subject.VoiceRelease, error) {
	var config domain.ProviderConfig
	if json.Unmarshal(r.Config, &config) != nil || config.Validate() != nil || config.AdapterKey != "volcengine_speech" {
		return subject.VoiceRelease{}, subject.ErrVoiceUnavailable
	}
	var cost struct {
		Currency, Source string
		ReviewedAt       string `json:"reviewed_at"`
		MaxMicros        int64  `json:"max_micros"`
	}
	if json.Unmarshal(r.UpstreamCost, &cost) != nil || (cost.Currency != "CNY" && cost.Currency != "USD") || strings.TrimSpace(cost.Source) == "" || cost.MaxMicros < 0 || cost.MaxMicros > 1_000_000_000 {
		return subject.VoiceRelease{}, subject.ErrVoiceUnavailable
	}
	if reviewed, err := time.Parse(time.RFC3339, cost.ReviewedAt); err != nil || reviewed.IsZero() {
		return subject.VoiceRelease{}, subject.ErrVoiceUnavailable
	}
	options, defaultKey, err := speechOptions(r.ParamSchema)
	if err != nil {
		return subject.VoiceRelease{}, err
	}
	return subject.VoiceRelease{VoiceModelOption: subject.VoiceModelOption{ReleaseID: r.ReleaseID, ModelVersionID: r.ModelVersionID, ModelKey: r.ModelKey, DisplayName: r.DisplayName, Voices: options, DefaultVoiceKey: defaultKey}, ProviderID: r.ProviderID, ConfigVersionID: r.ConfigVersionID, CredentialID: r.CredentialID, PriceVersionID: r.PriceVersionID, ResourceID: r.ResourceID, Moderation: r.Moderation, UpstreamCost: r.UpstreamCost}, nil
}

// List returns only actual active speech releases, never onboarding drafts.
func (s *SubjectVoiceCatalog) List(ctx context.Context) ([]subject.VoiceRelease, error) {
	if s == nil || s.db == nil {
		return nil, subject.ErrVoiceUnavailable
	}
	var rows []subjectSpeechRow
	if err := s.db.WithContext(ctx).Raw(subjectSpeechQuery + ` ORDER BY m.model_key,m.id LIMIT 120 FOR SHARE OF m,p,key,preset`).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := []subject.VoiceRelease{}
	for _, row := range rows {
		release, err := row.release()
		if err != nil {
			continue
		}
		out = append(out, release)
	}
	return out, nil
}

// Select freezes one explicit speaker from an exact currently active release.
func (s *SubjectVoiceCatalog) Select(ctx context.Context, id uuid.UUID, voice string) (subject.VoiceRelease, error) {
	var row subjectSpeechRow
	r := s.db.WithContext(ctx).Raw(subjectSpeechQuery+` AND r.id=? FOR SHARE OF m,p,key,preset`, id).Scan(&row)
	if r.Error != nil {
		return subject.VoiceRelease{}, r.Error
	}
	if r.RowsAffected != 1 {
		return subject.VoiceRelease{}, subject.ErrVoiceUnavailable
	}
	out, err := row.release()
	if err != nil || !slices.ContainsFunc(out.Voices, func(o subject.VoiceOption) bool { return o.Key == voice }) {
		return subject.VoiceRelease{}, subject.ErrVoiceUnavailable
	}
	return out, nil
}

// Credential loads only the original envelope, including a rotated draining key.
// Emergency or credential disable is checked again before any supplier call.
func (s *SubjectVoiceCatalog) Credential(ctx context.Context, work subject.VoiceWork) (subject.VoiceCredential, error) {
	var row struct {
		ProviderID, CredentialID uuid.UUID
		KeyID                    string
		Ciphertext, Config       []byte
	}
	r := s.db.WithContext(ctx).Raw(`SELECT p.id AS provider_id,key.id AS credential_id,key.key_id,key.ciphertext,config.config
 FROM catalog.model_release r JOIN catalog.model_profile m ON m.id=r.model_id
 JOIN catalog.provider p ON p.id=m.provider_id JOIN catalog.model_profile_version v ON v.id=r.model_version_id
 JOIN catalog.provider_config_version config ON config.id=r.config_version_id AND config.provider_id=p.id
 JOIN catalog.provider_preset_version pv ON pv.id=config.preset_version_id JOIN catalog.provider_preset preset ON preset.id=pv.preset_id
 JOIN catalog.provider_credential key ON key.provider_id=p.id
 WHERE r.id=? AND p.id=? AND config.id=? AND key.id=? AND v.id=? AND r.price_version_id=?
 AND r.status IN ('active','retired') AND NOT m.is_delete AND m.status='active' AND NOT p.is_delete AND p.status='active' AND p.adapter_key='volcengine_speech'
 AND v.provider_model_id='seed-tts-2.0' AND v.moderation='provider' AND v.queue='provider' AND (m.capability='audio.generate' AND v.modes=ARRAY['text2audio']::text[] OR m.capability='audio.tts' AND v.modes=ARRAY['tts']::text[]) AND EXISTS(SELECT 1 FROM catalog.model_validation validation JOIN operation.operation op ON op.id=validation.operation_id AND op.status='completed' AND NOT op.is_delete JOIN operation.provider_binding execution ON execution.operation_id=op.id AND execution.release_id=r.id JOIN operation.provider_credential_binding used_key ON used_key.operation_id=op.id AND used_key.credential_id=key.id WHERE validation.release_id=r.id AND validation.credential_id=key.id AND validation.status='succeeded' AND validation.mode=ANY(v.modes) AND validation.adapter_version='ai-provider-v1' AND validation.evidence->>'kind'='real_provider' AND op.mode=validation.mode AND op.model_profile_version_id=r.model_version_id AND op.price_rule_version_id=r.price_version_id) AND key.status IN ('active','draining') AND NOT key.is_delete AND NOT preset.emergency_disabled AND NOT preset.archived AND NOT v.is_delete
 FOR SHARE OF p,key,preset,m`, work.Job.ReleaseID, work.ProviderID, work.ConfigVersionID, work.CredentialID, work.Job.ModelVersionID, work.PriceVersionID).Scan(&row)
	if r.Error != nil {
		return subject.VoiceCredential{}, r.Error
	}
	if r.RowsAffected != 1 {
		return subject.VoiceCredential{}, subject.ErrVoiceUnavailable
	}
	var config domain.ProviderConfig
	if json.Unmarshal(row.Config, &config) != nil || config.Validate() != nil || config.AdapterKey != "volcengine_speech" {
		return subject.VoiceCredential{}, subject.ErrVoiceUnavailable
	}
	return subject.VoiceCredential{ProviderID: row.ProviderID, CredentialID: row.CredentialID, KeyID: row.KeyID, Ciphertext: row.Ciphertext, Config: row.Config}, nil
}

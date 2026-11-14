package postgres

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// instantiatePresetModels shares the connection's outer transaction and receipt.
// Nothing becomes public and no validation or default is copied from the source.
func (s *Store) instantiatePresetModels(ctx context.Context, tx *gorm.DB, actor identityapp.Principal, providerID, configID uuid.UUID, doc domain.PresetDocument) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	for _, template := range doc.ModelTemplates {
		id := uuid.New()
		model := domain.ModelProfile{ID: id, Key: "preset-" + id.String(), ProviderID: providerID, Capability: template.Capability, DisplayName: template.Name, Status: domain.ModelDisabled, Revision: 1}
		if err := createModelInTx(tx, model); err != nil {
			return nil, err
		}
		version := template.Version(id, actor.ID)
		if err := appendModelVersionInTx(tx, version, 1); err != nil {
			return nil, err
		}
		price := template.PriceVersion(id, actor.ID, time.Now().UTC())
		if price.ValidateProvenance() != nil || price.ReviewedAt.After(time.Now()) {
			return nil, domain.ErrInvalidPreset
		}
		if err := appendPriceRuleInTx(tx, price, 2); err != nil {
			return nil, err
		}
		cost := template.UpstreamCost
		_, err := application.NewModelReleases(s.withDB(tx)).Create(ctx, actor, id, application.CreateReleaseInput{ModelVersionID: version.ID, PriceVersionID: price.ID, ConfigVersionID: configID, UpstreamCost: application.UpstreamCost{Currency: cost.Currency, MaxMicros: cost.MaxMicros, Source: cost.Source, ReviewedAt: cost.ReviewedAt}, RequestID: uuid.New()})
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func connectionPresetDocument(tx *gorm.DB, config application.ConnectionConfig) (domain.PresetDocument, error) {
	if config.PresetVersionID == uuid.Nil {
		return domain.PresetDocument{SchemaVersion: 1, Config: config.Config}, nil
	}
	var payload []byte
	err := tx.Raw(`SELECT payload FROM catalog.provider_preset_version WHERE id=?`, config.PresetVersionID).Row().Scan(&payload)
	if err != nil {
		return domain.PresetDocument{}, err
	}
	var document domain.PresetDocument
	if err := json.Unmarshal(payload, &document); err != nil {
		return domain.PresetDocument{}, err
	}
	document.Config = config.Config
	return document, nil
}

// AddPresetModel serializes additions per provider and reuses an existing model
// for the same upstream model and capability.
func (s *Store) AddPresetModel(ctx context.Context, actor identityapp.Principal, id uuid.UUID, input application.AddPresetModelInput) (application.CreatedModel, error) {
	return catalogMutation(ctx, s.db, s.actors, actor, "model.preset.add", id, input.RequestID, input, func(tx *gorm.DB) (application.CreatedModel, error) {
		var empty application.CreatedModel
		var provider struct{ ID uuid.UUID }
		r := tx.Raw(`SELECT id FROM catalog.provider WHERE id=? AND status<>'archived' AND NOT is_delete FOR UPDATE`, id).Scan(&provider)
		if r.Error != nil {
			return empty, r.Error
		}
		if r.RowsAffected != 1 {
			return empty, application.ErrProviderNotFound
		}
		config, _, err := readConnectionConfig(tx, id)
		if err != nil {
			return empty, err
		}
		if config.ID != input.ConfigID {
			return empty, application.ErrPresetConflict
		}
		var credential bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM catalog.provider_credential WHERE provider_id=? AND status='active' AND NOT is_delete AND (?::uuid IS NULL OR id=?))`, id, input.CredentialID, input.CredentialID).Row().Scan(&credential); err != nil {
			return empty, err
		}
		if !credential {
			return empty, application.ErrCredentialNotFound
		}
		if input.DirectoryCheckID != nil {
			var current bool
			if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM catalog.provider_check check_run
 JOIN catalog.provider_credential c ON c.id=check_run.credential_id AND c.provider_id=check_run.provider_id AND c.status='active' AND NOT c.is_delete
 JOIN catalog.model_candidate candidate ON candidate.check_id=check_run.id
 WHERE check_run.id=? AND check_run.provider_id=? AND check_run.config_version_id=? AND check_run.credential_id=?
 AND check_run.kind='discovery' AND check_run.status='completed' AND check_run.discovery_result IN ('ok','partial')
 AND candidate.provider_model_id=?)`, input.DirectoryCheckID, id, config.ID, input.CredentialID, strings.TrimSpace(input.CustomModelID)).Scan(&current).Error; err != nil {
				return empty, err
			}
			if !current {
				return empty, application.ErrPresetConflict
			}
		}
		document, err := connectionPresetDocument(tx, config)
		if err != nil {
			return empty, err
		}
		catalog, err := readModelCatalog(tx, config.PresetVersionID, document)
		if err != nil {
			return empty, err
		}
		if config.PresetVersionID == uuid.Nil {
			catalog.Version = config.ID.String()
		}
		if input.CustomModelID != "" || config.PresetVersionID == uuid.Nil || config.Config.IsCustomGateway() {
			input.DraftOnly = true
		}
		candidate, err := resolveAddModelCandidate(config.Config, catalog, input)
		if err != nil {
			return empty, err
		}
		if candidate.RequiresModelSelection {
			return empty, domain.ErrInvalidPreset
		}
		var existing modelProfileRow
		r = tx.Raw(adminModelSelect+` WHERE provider_id=? AND capability=? AND NOT is_delete
			AND (candidate_snapshot->'model'->>'provider_model_id'=? OR current_version_id IN
				(SELECT id FROM catalog.model_profile_version WHERE provider_model_id=? AND NOT is_delete))
			ORDER BY (current_version_id IS NOT NULL) DESC, create_time, id LIMIT 1 FOR UPDATE`,
			id, candidate.Capability, candidate.ProviderModelID, candidate.ProviderModelID).Scan(&existing)
		if r.Error != nil {
			return empty, r.Error
		}
		if existing.CurrentVersionID != nil || (input.DraftOnly && existing.ID != uuid.Nil) {
			return createdModel(existing), nil
		}
		if input.DraftOnly {
			ids, err := saveSelectedModelDrafts(tx, actor, id, catalog.Version, document.ModelTemplates, []domain.ModelCandidatePreset{candidate})
			if err != nil {
				return empty, err
			}
			row, err := lockModelProfile(tx, ids[0])
			return createdModel(row), err
		}
		modelID := existing.ID
		expectedRevision := existing.Revision
		if modelID == uuid.Nil {
			for _, template := range document.ModelTemplates {
				if template.Key == candidate.Key {
					document.ModelTemplates = []domain.PresetModelTemplate{template}
					ids, err := s.instantiatePresetModels(ctx, tx, actor, id, config.ID, document)
					if err != nil {
						return empty, err
					}
					row, err := lockModelProfile(tx, ids[0])
					return createdModel(row), err
				}
			}
			expectedRevision = 1
			modelID, err = createPresetCandidate(tx, id, catalog.Version, candidate)
			if err != nil {
				return empty, err
			}
		}
		// A hand-entered ID without a persisted execution contract remains configurable,
		// but cannot acquire guessed model-specific defaults or an executable version.
		if candidate.Execution == nil {
			row, err := lockModelProfile(tx, modelID)
			return createdModel(row), err
		}
		if err := instantiateOnboardingModel(tx, actor, modelID, expectedRevision, candidate); err != nil {
			return empty, err
		}
		row, err := lockModelProfile(tx, modelID)
		return createdModel(row), err
	})
}

func createPresetCandidate(tx *gorm.DB, providerID uuid.UUID, catalogVersion string, candidate domain.ModelCandidatePreset) (uuid.UUID, error) {
	id := uuid.New()
	model := domain.ModelProfile{ID: id, Key: "candidate-" + id.String(), ProviderID: providerID, Capability: candidate.Capability, DisplayName: candidate.DisplayName, Status: domain.ModelDisabled, Revision: 1}
	if err := createModelInTx(tx, model); err != nil {
		return uuid.Nil, err
	}
	snapshot, err := json.Marshal(domain.ModelCandidateSelection{CatalogVersion: catalogVersion, Model: candidate})
	if err != nil {
		return uuid.Nil, err
	}
	return id, tx.Exec(`UPDATE catalog.model_profile SET candidate_snapshot=?::jsonb WHERE id=?`, string(snapshot), id).Error
}

func resolveAddModelCandidate(config domain.ProviderConfig, catalog domain.ModelCandidateCatalog, input application.AddPresetModelInput) (domain.ModelCandidatePreset, error) {
	var empty domain.ModelCandidatePreset
	if input.CatalogVersion != catalog.Version {
		return empty, application.ErrPresetConflict
	}
	if input.CandidateKey != "" {
		models, err := catalog.Select(input.CatalogVersion, []string{input.CandidateKey})
		if err != nil {
			return empty, err
		}
		return models[0], nil
	}

	id := strings.TrimSpace(input.CustomModelID)
	if id == "" || len(id) > 200 || strings.IndexFunc(id, func(r rune) bool {
		return unicode.IsControl(r) || unicode.IsSpace(r)
	}) >= 0 || !slices.Contains(config.Capabilities, strings.TrimSuffix(input.Capability, ".generate")) {
		return empty, domain.ErrInvalidPreset
	}
	name := strings.TrimSpace(input.CustomModelName)
	if name == "" {
		name = id
	}
	if input.DraftOnly {
		return domain.ModelCandidatePreset{
			Key: "custom-" + uuid.NewString(), AdapterKey: config.AdapterKey,
			DisplayName: name, ProviderModelID: id, Capability: input.Capability,
			SourceURL: "custom://admin", Lifecycle: "unknown",
		}, nil
	}

	var mode string
	switch input.Capability {
	case "text.generate":
		mode = "text_to_text"
	case "image.generate":
		mode = "text2image"
	case "video.generate":
		mode = "text2video"
	default:
		return empty, domain.ErrInvalidPreset
	}
	return domain.ModelCandidatePreset{
		Key: "custom-" + uuid.NewString(), AdapterKey: config.AdapterKey,
		DisplayName: name, ProviderModelID: id, Capability: input.Capability,
		Modes: []string{mode}, Lifecycle: "unknown",
	}, nil
}

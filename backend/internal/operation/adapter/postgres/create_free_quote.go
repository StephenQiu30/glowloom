package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	promptapp "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/application"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

// CreateFreeQuote keeps the current project, catalog and quote snapshot in one
// transaction. The amount is an estimate; confirmation locks and rechecks the
// mutable facts and budget before reserving any money.
func (s *Store) CreateFreeQuote(ctx context.Context, actor identityapp.Principal, input application.CreateFreeQuoteInput) (application.CreateFreeQuoteResult, error) {
	if s == nil || s.db == nil {
		return application.CreateFreeQuoteResult{}, ErrUnavailable
	}
	if len(input.Params) == 0 && input.InputSource == nil {
		input.Params = json.RawMessage(`{}`)
	}
	if err := input.Validate(); err != nil {
		return application.CreateFreeQuoteResult{}, err
	}
	requestID := uuid.MustParse(input.RequestID)
	fingerprint, err := quoteRequestFingerprint(input)
	if err != nil {
		return application.CreateFreeQuoteResult{}, err
	}
	var result application.CreateFreeQuoteResult
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireProject(tx, actor, input.ProjectID, true); err != nil {
			return err
		}
		if err := lockQuoteRequest(tx, actor, requestID); err != nil {
			return err
		}
		var editorSlot application.SubmissionCurrent
		if input.InputSource != nil {
			var slotErr error
			editorSlot, slotErr = s.lockEditorOwner(ctx, tx, actor, input.ProjectID, *input.InputSource)
			if slotErr != nil {
				return slotErr
			}
		}
		project, err := lockFreeQuoteProject(tx, actor, input.ProjectID)
		if err != nil {
			return err
		}
		stored, found, err := readQuoteRequest(tx, actor, requestID, fingerprint)
		if err != nil {
			return err
		}
		if found {
			if err := json.Unmarshal(stored, &result); err != nil {
				return fmt.Errorf("decode stored free quote: %w", err)
			}
			if result.OperationID == uuid.Nil || result.ExpiresAt.IsZero() || result.QuoteMicros < 0 {
				return fmt.Errorf("decode stored free quote: invalid result shape")
			}
			return nil
		}
		var editorDraft *promptdomain.Draft
		originalSource := input.InputSource
		if originalSource != nil {
			if editorSlot.Submission != nil {
				return application.ErrSubmissionUnresolved
			}
			if input.ExpectedSubmissionEpoch == nil || editorSlot.Epoch != *input.ExpectedSubmissionEpoch {
				return application.ErrSubmissionEpoch
			}
			draft, readErr := s.editorSnapshots(tx).ReadInput(ctx, actor, input.ProjectID, *originalSource, false)
			if readErr != nil {
				return readErr
			}
			editorDraft = &draft
			prepared, prepareErr := compileEditorInput(tx, input, draft)
			if prepareErr != nil {
				return prepareErr
			}
			input = prepared
		}
		now := time.Now().UTC()
		item, _, err := s.prepareFreeQuoteItem(ctx, tx, actor, input, project, now)
		if err != nil {
			return err
		}
		if editorDraft != nil && (item.Operation.ModelProfileVersionID == nil || item.Operation.ModelProfileVersionID.String() != *editorDraft.ModelVersion) {
			return application.ErrFreeQuoteModelMissing
		}
		budget, err := s.billingStore(tx).FindBudget(ctx, actor, input.ProjectID)
		if err != nil {
			return err
		}
		available, err := budget.AvailableMicros()
		if err != nil {
			return err
		}
		if err := s.withDB(tx).CreateQuoteSnapshot(ctx, actor, nil, []QuoteItem{item}); err != nil {
			return err
		}
		result = application.CreateFreeQuoteResult{
			OperationID: item.Operation.ID, QuoteMicros: *item.Operation.QuoteMicros,
			QuoteDetail:     item.Operation.QuoteDetail,
			AvailableMicros: available, ExpiresAt: *item.Operation.QuoteExpiresAt,
			ReusedFromID:      item.Operation.ReusedFromID,
			Confirmable:       available >= *item.Operation.QuoteMicros,
			PromptPreparation: item.PromptPreparation,
		}
		if item.PromptPreparation != nil || editorDraft != nil {
			result.FinalPrompt = *item.Inputs[0].TextValue
		}
		if originalSource != nil {
			if err := recordEditorSubmission(tx, actor, input.ProjectID, *originalSource, editorSlot.Epoch, *editorDraft, &result); err != nil {
				return err
			}
		}
		return insertQuoteRequest(tx, actor, requestID, fingerprint, result)
	})
	if err != nil {
		return application.CreateFreeQuoteResult{}, fmt.Errorf("create free quote transaction: %w", err)
	}
	return result, nil
}

type freeQuoteProject struct {
	AllowOverseasModels bool
	DefaultModels       string
	Name                string
	StyleType           string
}

func lockFreeQuoteProject(tx *gorm.DB, actor identityapp.Principal, projectID uuid.UUID) (freeQuoteProject, error) {
	var project freeQuoteProject
	read := tx.Raw(`
		SELECT allow_overseas_models, default_models::text AS default_models,name,style_type FROM workspace.project
		WHERE id = ?::uuid AND org_id = ?::uuid
		  AND status = 'active' AND NOT is_delete FOR SHARE
	`, projectID.String(), actor.OrgID.String()).Scan(&project)
	if read.Error != nil {
		return freeQuoteProject{}, fmt.Errorf("lock free quote project: %w", read.Error)
	}
	if read.RowsAffected != 1 {
		return freeQuoteProject{}, ErrNotFound
	}
	return project, nil
}

func (s *Store) prepareFreeQuoteItem(ctx context.Context, tx *gorm.DB, actor identityapp.Principal,
	input application.CreateFreeQuoteInput, project freeQuoteProject, now time.Time,
) (QuoteItem, string, error) {
	if len(input.Params) == 0 && input.InputSource == nil {
		input.Params = json.RawMessage(`{}`)
	}
	if err := input.Validate(); err != nil {
		return QuoteItem{}, "", err
	}
	modelKey, err := resolveEffectiveQuoteModelKey(tx, input.FreeQuoteItemInput, project)
	if err != nil {
		return QuoteItem{}, "", err
	}
	input.ModelKey = modelKey
	if _, err := readQuoteCanvasSource(tx, input.ProjectID, input.FreeQuoteItemInput); err != nil {
		return QuoteItem{}, "", err
	}
	var compilation *promptapp.Preparation
	if input.PromptTemplate != nil {
		if s.promptCompiler == nil {
			return QuoteItem{}, "", promptapp.ErrTemplateUnavailable
		}
		compiler := s.promptCompiler(tx)
		if compiler == nil {
			return QuoteItem{}, "", promptapp.ErrTemplateUnavailable
		}
		preparedPrompt, err := compiler.Prepare(ctx, actor, *input.PromptTemplate, promptapp.RuntimeContext{
			ProjectID: input.ProjectID, Capability: input.Capability, Mode: input.Mode, UserPrompt: input.Prompt,
			ServerValues: map[string]string{"项目名称": project.Name, "项目画风": project.StyleType},
		})
		if err != nil {
			return QuoteItem{}, "", err
		}
		compilation = &preparedPrompt
	}
	catalog, err := readFreeQuoteCatalog(tx, input, project.AllowOverseasModels, now)
	if err != nil {
		return QuoteItem{}, "", err
	}
	media, err := readFreeQuoteMedia(tx, input.ProjectID, input.MediaInputs)
	if err != nil {
		return QuoteItem{}, "", err
	}
	prepared, err := application.PrepareFreeQuoteWithTemplate(input, catalog, media, now, nil, compilation)
	if err != nil {
		return QuoteItem{}, "", err
	}
	if prepared.Reusable && !input.ForceRegenerate {
		candidateID, findErr := s.withDB(tx).FindReusableCompleted(ctx, actor, input.ProjectID, prepared.Operation.InputHash)
		if findErr != nil && !errors.Is(findErr, ErrNotFound) {
			return QuoteItem{}, "", findErr
		}
		if findErr == nil {
			prepared, err = application.PrepareFreeQuoteWithTemplate(input, catalog, media, now, &candidateID, compilation)
			if err != nil {
				return QuoteItem{}, "", err
			}
		}
	}
	return QuoteItem{Operation: prepared.Operation, Inputs: prepared.Inputs, PromptPreparation: prepared.PromptPreparation}, modelKey, nil
}

// A project selection always wins; an unavailable explicit selection never
// silently routes a paid request to another model.
func resolveEffectiveQuoteModelKey(tx *gorm.DB, input application.FreeQuoteItemInput, project freeQuoteProject) (string, error) {
	if input.ModelKey != "" {
		return input.ModelKey, nil
	}
	var defaults map[string]json.RawMessage
	if err := json.Unmarshal([]byte(project.DefaultModels), &defaults); err != nil || defaults == nil {
		return "", application.ErrFreeQuoteModelMissing
	}
	if _, exists := defaults[input.Capability]; exists {
		return resolveFreeQuoteModelKey(input, project)
	}
	var row struct{ Key string }
	r := tx.Raw(`SELECT m.model_key AS key FROM catalog.capability_default d
 JOIN catalog.model_release r ON r.id=d.release_id AND r.status='active'
 JOIN catalog.model_profile m ON m.id=r.model_id AND m.active_release_id=r.id
 WHERE d.capability=? AND m.capability=d.capability AND m.status='active' AND NOT m.is_delete FOR SHARE OF d,m,r`, input.Capability).Scan(&row)
	if r.Error != nil {
		return "", r.Error
	}
	if r.RowsAffected != 1 {
		return "", application.ErrFreeQuoteModelMissing
	}
	return row.Key, nil
}

func resolveFreeQuoteModelKey(input application.FreeQuoteItemInput, project freeQuoteProject) (string, error) {
	if input.ModelKey != "" {
		return input.ModelKey, nil
	}
	var defaults map[string]json.RawMessage
	if err := json.Unmarshal([]byte(project.DefaultModels), &defaults); err != nil || defaults == nil {
		return "", application.ErrFreeQuoteModelMissing
	}
	var modelKey string
	if err := json.Unmarshal(defaults[input.Capability], &modelKey); err != nil || modelKey == "" {
		return "", application.ErrFreeQuoteModelMissing
	}
	return modelKey, nil
}

type freeQuoteCatalogRow struct {
	ReleaseID          uuid.UUID
	ModelID            uuid.UUID
	VersionID          uuid.UUID
	ProviderModelID    string
	Region             string
	Modes              string
	InputRoles         string
	Limits             string
	ParamSchema        string
	PriceID            uuid.UUID
	PriceVersionNo     int
	PriceUnit          string
	PriceRule          string
	PriceCurrency      string
	PriceFXRateToCNY   *string
	PriceEffectiveFrom time.Time
}

func readFreeQuoteCatalog(tx *gorm.DB, input application.CreateFreeQuoteInput, allowOverseas bool, now time.Time) (application.FreeQuoteCatalog, error) {
	var row freeQuoteCatalogRow
	read := tx.Raw(`
		SELECT published.id AS release_id, m.id AS model_id, v.id AS version_id,
		       v.provider_model_id, provider.region,
		       to_jsonb(v.modes)::text AS modes,
		       to_jsonb(capability.input_roles)::text AS input_roles,
		       v.limits::text AS limits, v.param_schema::text AS param_schema,
		       price.id AS price_id, price.version_no AS price_version_no,
		       price.unit AS price_unit, price.rule::text AS price_rule,
		       price.currency::text AS price_currency,
		       price.fx_rate_to_cny::text AS price_fx_rate_to_cny,
		       price.effective_from AS price_effective_from
		FROM catalog.model_profile AS m
		JOIN catalog.provider AS provider
		  ON provider.id = m.provider_id AND provider.status = 'active' AND NOT provider.is_delete
		JOIN catalog.provider_credential AS credential
		  ON credential.provider_id = provider.id
		 AND credential.status = 'active' AND NOT credential.is_delete
		LEFT JOIN catalog.model_release AS published ON published.id=m.active_release_id AND published.status='active'
		JOIN catalog.capability AS capability
		  ON capability.key = m.capability AND NOT capability.is_delete
		JOIN catalog.model_profile_version AS v
		  ON v.id = CASE WHEN provider.adapter_key IN ('volcengine_ark','openrouter') THEN published.model_version_id ELSE m.current_version_id END AND v.model_profile_id = m.id AND NOT v.is_delete
		JOIN LATERAL (
		  SELECT id, version_no, unit, rule, currency, fx_rate_to_cny, effective_from
		  FROM catalog.price_rule_version
		  WHERE (provider.adapter_key NOT IN ('volcengine_ark','openrouter') OR id=published.price_version_id) AND model_profile_id = m.id AND effective_from <= ? AND NOT is_delete
		  ORDER BY effective_from DESC, version_no DESC LIMIT 1
		) AS price ON true
		WHERE m.model_key = ? AND (provider.adapter_key NOT IN ('volcengine_ark','openrouter') OR EXISTS (SELECT 1 FROM catalog.provider_config_version pc JOIN catalog.provider_preset_version pv ON pv.id=pc.preset_version_id JOIN catalog.provider_preset preset ON preset.id=pv.preset_id WHERE pc.id=published.config_version_id AND NOT preset.emergency_disabled)) AND m.capability = ? AND m.status = 'active'
		  AND NOT m.is_delete AND ? = ANY(v.modes) AND ? = ANY(capability.modes)
		  AND 'prompt' = ANY(capability.input_roles)
		  AND (? OR provider.region = 'domestic')
		FOR SHARE OF m, provider, credential
	`, now, input.ModelKey, input.Capability, input.Mode, input.Mode, allowOverseas).Scan(&row)
	if read.Error != nil {
		return application.FreeQuoteCatalog{}, fmt.Errorf("read current free quote model: %w", read.Error)
	}
	if read.RowsAffected != 1 {
		return application.FreeQuoteCatalog{}, application.ErrFreeQuoteModelMissing
	}
	return freeCatalogFromRow(row)
}

func freeCatalogFromRow(row freeQuoteCatalogRow) (application.FreeQuoteCatalog, error) {
	var modes []string
	if err := json.Unmarshal([]byte(row.Modes), &modes); err != nil {
		return application.FreeQuoteCatalog{}, fmt.Errorf("decode free quote model modes: %w", err)
	}
	var roles []string
	if err := json.Unmarshal([]byte(row.InputRoles), &roles); err != nil {
		return application.FreeQuoteCatalog{}, fmt.Errorf("decode free quote input roles: %w", err)
	}
	fx := ""
	if row.PriceFXRateToCNY != nil {
		fx = *row.PriceFXRateToCNY
	}
	return application.FreeQuoteCatalog{
		ReleaseID: row.ReleaseID, ModelVersionID: row.VersionID, ProviderModelID: row.ProviderModelID,
		Region: row.Region, Modes: modes, InputRoles: roles,
		Limits: json.RawMessage(row.Limits), ParamSchema: json.RawMessage(row.ParamSchema),
		Price: catalogdomain.PriceRuleVersion{
			ID: row.PriceID, ModelID: row.ModelID, VersionNo: row.PriceVersionNo,
			Unit: catalogdomain.PriceUnit(row.PriceUnit), Rule: json.RawMessage(row.PriceRule),
			Currency: row.PriceCurrency, FXRateToCNY: fx,
			EffectiveFrom: row.PriceEffectiveFrom,
		},
	}, nil
}

func readFreeQuoteMedia(tx *gorm.DB, projectID uuid.UUID, requested []application.FreeQuoteMediaInput) ([]application.FreeQuoteMediaFact, error) {
	if len(requested) == 0 {
		return nil, nil
	}
	assetIDs := make(map[uuid.UUID]struct{}, len(requested))
	for _, input := range requested {
		assetIDs[input.MediaAssetID] = struct{}{}
	}
	ordered := make([]uuid.UUID, 0, len(assetIDs))
	for id := range assetIDs {
		ordered = append(ordered, id)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].String() < ordered[j].String() })
	facts := make(map[uuid.UUID]application.FreeQuoteMediaFact, len(ordered))
	for _, id := range ordered {
		var row struct {
			ID                 uuid.UUID
			Kind               string
			Status             string
			ModerationStatus   string
			ContainsRealPerson bool
			ConsentRecordID    *uuid.UUID
			SHA256             *string
			ByteSize           int64
			DurationMS         *int64
		}
		read := tx.Raw(`
			SELECT id, kind, status, moderation_status, contains_real_person,
			       consent_record_id, sha256, byte_size, duration_ms
			FROM media.media_asset
			WHERE id = ?::uuid AND project_id = ?::uuid AND NOT is_delete FOR SHARE
		`, id.String(), projectID.String()).Scan(&row)
		if read.Error != nil {
			return nil, fmt.Errorf("lock free quote media: %w", read.Error)
		}
		if read.RowsAffected != 1 || row.Status != "ready" || row.ModerationStatus != "passed" ||
			row.ContainsRealPerson || row.ConsentRecordID != nil || row.SHA256 == nil {
			return nil, application.ErrFreeQuoteInputNotReady
		}
		facts[id] = application.FreeQuoteMediaFact{
			ID: id, Kind: row.Kind, SHA256: *row.SHA256, ByteSize: row.ByteSize,
			DurationMS: row.DurationMS,
		}
	}
	result := make([]application.FreeQuoteMediaFact, 0, len(requested))
	for _, input := range requested {
		result = append(result, facts[input.MediaAssetID])
	}
	return result, nil
}

var _ application.FreeQuoteCreator = (*Store)(nil)

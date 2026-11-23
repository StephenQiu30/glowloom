package aigateway

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	catalog "github.com/StephenQiu30/glowloom-server/backend/execution/catalog/application"
	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/execution/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
)

// Catalog projects the new immutable model contracts for existing presentation consumers.
// It never reads the old catalog or confers new price/credential authority.
type Catalog struct {
	DB     *gorm.DB
	Worker bool
}

// ListModelsForProject lists enabled products admitted to the current project.
func (c Catalog) ListModelsForProject(ctx context.Context, a identity.Principal, in catalog.ListModelsInput) (catalog.ModelCatalogPage, error) {
	result := catalog.ModelCatalogPage{Models: []catalog.ModelCatalogItem{}}
	err := c.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if c.Worker {
			if _, err := scopewire.ProvideScopeAccess(tx).Resolve(ctx, a.ID, a.ScopeID, false); err != nil {
				return err
			}
			if err := projectScope(tx, a.ScopeID, in.ProjectID); err != nil {
				return err
			}
		} else if err := Authority(ctx, tx, a, a.ScopeID, in.ProjectID, false); err != nil {
			return err
		}
		query := `SELECT document,capability,route_id,credential_id FROM ai.available_model WHERE true`
		args := []any{}
		if in.Capability != "" {
			query += " AND capability=?"
			args = append(args, in.Capability)
		}
		if in.Mode != "" {
			query += " AND (capability='text.generate' AND ?='text_to_text' OR document->>'protocol'='media' AND document->'modes' @> jsonb_build_array(?::text))"
			args = append(args, in.Mode, in.Mode)
		}
		if in.ModelKey != "" {
			query += " AND name=?"
			args = append(args, in.ModelKey)
		}
		if in.After != nil {
			query += " AND (name,id)>(?,?)"
			args = append(args, in.After.Key, in.After.ID)
		}
		limit := in.Limit
		if limit < 1 {
			limit = 50
		}
		query += " ORDER BY name,id LIMIT ?"
		args = append(args, limit+1)
		var rows []struct {
			Document              []byte
			Capability            string
			RouteID, CredentialID uuid.UUID
		}
		if err := tx.Raw(query, args...).Scan(&rows).Error; err != nil {
			return err
		}
		for i, row := range rows {
			if i == limit {
				last := result.Models[i-1]
				result.Next = &catalog.ModelCatalogCursor{Key: last.Key, ID: last.ID}
				break
			}
			var m domain.Model
			if json.Unmarshal(row.Document, &m) != nil || m.Validate() != nil {
				return domain.ErrInvalid
			}
			capability := row.Capability
			modes := []string{"text_to_text"}
			roles := []string{"prompt"}
			if m.Protocol == "media" {
				modes = m.Modes
				roles = []string{"prompt", "reference_image", "first_frame", "last_frame", "reference_video", "reference_audio"}
			}
			schema, _ := json.Marshal([]map[string]any{{"field": "max_tokens", "label": "最大输出", "type": "integer", "component": "input", "min": 1, "max": m.MaxOutput, "default": min(int64(8192), m.MaxOutput)}})
			if m.Protocol == "media" {
				fields := []map[string]any{}
				names := []string{}
				for name := range m.Parameters {
					names = append(names, name)
				}
				sort.Strings(names)
				for _, name := range names {
					p := m.Parameters[name]
					f := map[string]any{"field": name, "label": name, "type": p.Type, "component": "input"}
					if p.Min != nil {
						f["min"] = *p.Min
					}
					if p.Max != nil {
						f["max"] = *p.Max
					}
					if p.Default != nil {
						f["default"] = p.Default
					}
					if len(p.Enum) > 0 {
						f["enum"] = p.Enum
						f["component"] = "select"
					}
					fields = append(fields, f)
				}
				schema, _ = json.Marshal(fields)
			}
			// This display price is not accepted for charging. All actual quotes come
			// from the new money contract and are frozen in ai.request.
			input, _ := m.Price.Cost(domain.Usage{Input: 1000})
			output, _ := m.Price.Cost(domain.Usage{Output: 1000})
			price, _ := json.Marshal(map[string]any{"input_micros_per_1k": input, "output_micros_per_1k": output, "gateway_quote_required": true})
			unit := "per_1k_tokens"
			if m.Protocol == "media" {
				unit = "per_request"
				amount, _ := m.Price.Cost(domain.Usage{})
				price, _ = json.Marshal(map[string]any{"micros": amount, "gateway_quote_required": true})
			}
			region := catalogdomain.RegionOverseas
			limits := json.RawMessage(`{"max_outputs":1}`)
			window := m.MaxInput + m.MaxOutput
			var availability catalog.ModelAvailability
			if capability == domain.EmbeddingCapability {
				region = catalogdomain.RegionDomestic
				modes, roles = []string{}, []string{"text"}
				schema = json.RawMessage("[]")
				limits, _ = json.Marshal(map[string]int{"dimensions": domain.EmbeddingDimensions})
				window = m.MaxInput
				// Vector releases have a qualified route and no generation modes.
				availability = catalog.ModelAvailability{Available: true, ReleaseID: m.ID, ModelVersionID: m.ID,
					PriceVersionID: m.ID, ConfigVersionID: row.RouteID, CredentialID: row.CredentialID,
					ModeAvailability: []catalog.ModeAvailability{}, UnavailableReasons: []string{}}
			}
			item := catalog.ModelCatalogItem{ModelAvailability: availability, ID: m.ID, ReleaseID: m.ID, Key: m.Name, DisplayName: m.Name, Capability: capability, InputRoles: roles, Status: catalogdomain.ModelStatus("active"), ProviderID: m.ID, ProviderName: "AI网关", ProviderAdapterKey: m.Protocol, ProviderRegion: region, ProviderStatus: catalogdomain.ProviderStatus("active"), CredentialPresent: true,
				CurrentVersion: &catalog.ModelCatalogVersion{ID: m.ID, VersionNo: 1, ContextWindow: int(window), Modes: modes, Limits: limits, ParamSchema: schema},
				CurrentPrice:   &catalog.ModelCatalogPrice{ID: m.ID, VersionNo: 1, Unit: unit, Rule: price, Currency: "CNY", EffectiveFrom: time.Unix(0, 0)}}

			result.Models = append(result.Models, item)
		}
		return nil
	})
	return result, err
}

var _ catalog.ListModelsStore = Catalog{}

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	platformdb "github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
)

// ListProvidersForAdmin reads one keyset page with safe credential columns and
// counts non-deleted model profiles after checking current administrator rights.
func (s *Store) ListProvidersForAdmin(ctx context.Context, actor identityapp.Principal, input application.ListProvidersInput) (application.ProviderListPage, error) {
	limit, after := input.Limit, input.After
	if s == nil || s.db == nil || actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || limit < 1 || limit > 200 ||
		(after != nil && (after.ID == uuid.Nil || after.CreateTime.IsZero())) {
		return application.ProviderListPage{}, application.ErrInvalidProviderList
	}
	var page application.ProviderListPage
	err := platformdb.ReadSnapshot(ctx, s.db, func(tx *gorm.DB) error {
		page = application.ProviderListPage{Providers: []application.ProviderListItem{}}
		if err := s.requirePlatformAdmin(tx, actor); err != nil {
			return err
		}
		query := `
			SELECT p.id AS provider_id, p.key, p.name, p.adapter_key, p.region,
			       p.status AS provider_status, p.concurrency_limit, p.rate_limit_per_min,
			       p.revision, p.create_time AS provider_create_time,
			       p.update_time AS provider_update_time,
			       c.id AS credential_id, c.label AS credential_label,
			       c.last4 AS credential_last4, c.status AS credential_status,
			       c.last_tested_at, c.last_test_result,
			       c.create_time AS credential_create_time,
			       c.update_time AS credential_update_time,
			       (SELECT count(*)::integer FROM catalog.model_profile AS m
			        WHERE m.provider_id = p.id AND NOT m.is_delete) AS model_count
			FROM catalog.provider AS p
			LEFT JOIN catalog.provider_credential AS c
			  ON c.provider_id = p.id AND c.status = 'active' AND NOT c.is_delete
			WHERE NOT p.is_delete
		`
		args := []any{}
		if input.Name != "" {
			query += ` AND (strpos(lower(p.name), lower(?)) > 0 OR p.id::text=lower(?) OR p.key=?)`
			args = append(args, input.Name, input.Name, input.Name)
		}
		if input.AdapterKey != "" {
			query += ` AND p.adapter_key = ?`
			args = append(args, input.AdapterKey)
		}
		if after != nil {
			query += ` AND (p.create_time, p.id) < (?::timestamptz, ?::uuid)`
			args = append(args, after.CreateTime.UTC(), after.ID)
		}
		baseQuery, baseArgs := query, append([]any{}, args...)
		var scanAfter *application.ProviderListCursor
		more := false
		page.Providers = []application.ProviderListItem{}
		for {
			batchQuery, batchArgs := baseQuery, append([]any{}, baseArgs...)
			if scanAfter != nil {
				batchQuery += ` AND (p.create_time,p.id)<(?::timestamptz,?::uuid)`
				batchArgs = append(batchArgs, scanAfter.CreateTime.UTC(), scanAfter.ID)
			}
			batchQuery += ` ORDER BY p.create_time DESC,p.id DESC LIMIT 200`
			var rows []providerListRow
			if err := tx.Raw(batchQuery, batchArgs...).Scan(&rows).Error; err != nil {
				return fmt.Errorf("read provider list: %w", err)
			}
			for _, row := range rows {
				item, err := row.item()
				if err != nil {
					return err
				}
				item.UnavailableReasons = []string{}
				// Bound model fact reads while aggregating a whole connection.
				var modelAfter uuid.UUID
				for {
					var ids []uuid.UUID
					if err := tx.Raw(`SELECT id FROM catalog.model_profile WHERE provider_id=? AND NOT is_delete AND id>?::uuid ORDER BY id LIMIT 200`, item.Provider.ID, modelAfter).Scan(&ids).Error; err != nil {
						return err
					}
					availability, err := readModelAvailability(tx, ids)
					if err != nil {
						return err
					}
					for _, state := range availability {
						if state.Available {
							item.AvailableModelCount++
						}
					}
					if len(ids) < 200 {
						break
					}
					modelAfter = ids[len(ids)-1]
				}
				item.Available = item.AvailableModelCount > 0
				if !item.Available {
					item.UnavailableReasons = append(item.UnavailableReasons, "no_available_model")
				}
				if input.Status != "" && item.Available != (input.Status == "available") {
					continue
				}
				page.Providers = append(page.Providers, item)
				if len(page.Providers) > limit {
					more = true
					break
				}
			}
			if more || len(rows) < 200 {
				break
			}
			last := rows[len(rows)-1]
			scanAfter = &application.ProviderListCursor{CreateTime: last.ProviderCreateTime, ID: last.ProviderID}
		}
		if more {
			page.Providers = page.Providers[:limit]
		}

		if more {
			last := page.Providers[len(page.Providers)-1].Provider
			page.Next = &application.ProviderListCursor{CreateTime: last.CreateTime, ID: last.ID}
		}
		return nil
	})
	if err != nil {
		return application.ProviderListPage{}, fmt.Errorf("list providers transaction: %w", err)
	}
	return page, nil
}

type providerListRow struct {
	ProviderID           uuid.UUID
	Key                  string
	Name                 string
	AdapterKey           string
	Region               string
	ProviderStatus       string
	ConcurrencyLimit     int
	RateLimitPerMin      int
	Revision             int64
	ProviderCreateTime   time.Time
	ProviderUpdateTime   time.Time
	CredentialID         *uuid.UUID
	CredentialLabel      *string
	CredentialLast4      *string
	CredentialStatus     *string
	LastTestedAt         *time.Time
	LastTestResult       *string
	CredentialCreateTime *time.Time
	CredentialUpdateTime *time.Time
	ModelCount           int
}

func (row providerListRow) item() (application.ProviderListItem, error) {
	item := application.ProviderListItem{
		Provider: application.CreatedProvider{
			ID: row.ProviderID, Key: row.Key, Name: row.Name, AdapterKey: row.AdapterKey,
			Region: domain.Region(row.Region), Status: domain.ProviderStatus(row.ProviderStatus),
			ConcurrencyLimit: row.ConcurrencyLimit, RateLimitPerMin: row.RateLimitPerMin,
			Revision: row.Revision, CreateTime: row.ProviderCreateTime, UpdateTime: row.ProviderUpdateTime,
		},
		ModelCount: row.ModelCount,
	}
	if row.CredentialID == nil {
		return item, nil
	}
	if row.CredentialLabel == nil || row.CredentialLast4 == nil || row.CredentialStatus == nil ||
		row.CredentialCreateTime == nil || row.CredentialUpdateTime == nil {
		return application.ProviderListItem{}, application.ErrInvalidProviderList
	}
	item.Credential = &application.ProviderCredentialSummary{
		ID: *row.CredentialID, ProviderID: row.ProviderID, Label: *row.CredentialLabel,
		Last4: *row.CredentialLast4, Status: domain.CredentialStatus(*row.CredentialStatus),
		LastTestedAt: row.LastTestedAt, CreateTime: *row.CredentialCreateTime,
		UpdateTime: *row.CredentialUpdateTime,
	}
	if row.LastTestResult != nil {
		result := domain.TestResult(*row.LastTestResult)
		item.Credential.LastTestResult = &result
	}
	return item, nil
}

var _ application.ListProvidersStore = (*Store)(nil)

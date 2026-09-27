package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
)

// ListProvidersForAdmin reads one keyset page with safe credential columns and
// counts non-deleted model profiles after checking current administrator rights.
func (s *Store) ListProvidersForAdmin(ctx context.Context, actorID, orgID uuid.UUID, limit int, after *application.ProviderListCursor) (application.ProviderListPage, error) {
	if s == nil || s.db == nil || actorID == uuid.Nil || orgID == uuid.Nil || limit < 1 || limit > 200 ||
		(after != nil && (after.ID == uuid.Nil || after.CreateTime.IsZero())) {
		return application.ProviderListPage{}, application.ErrInvalidProviderList
	}
	var page application.ProviderListPage
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentCatalogAdmin(tx, actorID, orgID); err != nil {
			return err
		}
		const baseQuery = `
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
		query := baseQuery + ` ORDER BY p.create_time DESC, p.id DESC LIMIT ?`
		args := []any{limit + 1}
		if after != nil {
			query = baseQuery + ` AND (p.create_time, p.id) < (?::timestamptz, ?::uuid)
				ORDER BY p.create_time DESC, p.id DESC LIMIT ?`
			args = []any{after.CreateTime.UTC(), after.ID.String(), limit + 1}
		}
		var rows []providerListRow
		if err := tx.Raw(query, args...).Scan(&rows).Error; err != nil {
			return fmt.Errorf("read provider list: %w", err)
		}
		more := len(rows) > limit
		if more {
			rows = rows[:limit]
		}
		page.Providers = make([]application.ProviderListItem, 0, len(rows))
		for _, row := range rows {
			item, err := row.item()
			if err != nil {
				return err
			}
			page.Providers = append(page.Providers, item)
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

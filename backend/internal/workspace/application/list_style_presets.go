package application

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

var (
	// ErrProjectNotFound hides projects outside the caller's current organization.
	ErrProjectNotFound = errors.New("project not found")
	// ErrStylePresetNotFound hides unavailable or out-of-scope presets.
	ErrStylePresetNotFound = errors.New("style preset not found")
	// ErrStylePresetMismatch means a visible preset has different specifications.
	ErrStylePresetMismatch = errors.New("style preset does not match project style")
	// ErrInvalidStylePresetList means a public preset filter or cursor is invalid.
	ErrInvalidStylePresetList = errors.New("invalid style preset list")
)

// StylePresetSummary excludes prompt contents, reference IDs and storage data.
type StylePresetSummary struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	StyleType    string    `json:"style_type"`
	StyleSubtype string    `json:"style_subtype,omitempty"`
}

// StylePresetCursor identifies the last name/ID pair in ascending order.
type StylePresetCursor struct {
	Name string
	ID   uuid.UUID
}

// ListStylePresetsInput filters organization-level presets available for creation.
type ListStylePresetsInput struct {
	StyleType    string
	StyleSubtype string
	Limit        int
	After        *StylePresetCursor
}

// StylePresetPage contains a bounded safe page, never synthetic default presets.
type StylePresetPage struct {
	Items []StylePresetSummary
	Next  *StylePresetCursor
}

// StylePresetSummaryStore reads only public metadata after rechecking the actor.
type StylePresetSummaryStore interface {
	ListStylePresetSummaries(context.Context, identityapp.Principal, ListStylePresetsInput) (StylePresetPage, error)
}

// ListStylePresetsQuery owns the validated organization preset query.
type ListStylePresetsQuery struct{ store StylePresetSummaryStore }

// NewListStylePresetsQuery injects the safe authorized reader.
func NewListStylePresetsQuery(store StylePresetSummaryStore) *ListStylePresetsQuery {
	return &ListStylePresetsQuery{store: store}
}

// Execute returns usable organization presets, excluding project-scoped rows.
func (q *ListStylePresetsQuery) Execute(ctx context.Context, actor identityapp.Principal, input ListStylePresetsInput) (StylePresetPage, error) {
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.MustChangePassword ||
		(actor.Role != identitydomain.RoleAdmin && actor.Role != identitydomain.RoleProducer) {
		return StylePresetPage{}, identityapp.ErrForbidden
	}
	if q == nil || q.store == nil || input.Limit < 0 || input.Limit > 200 ||
		(input.StyleType != "" && input.StyleType != "realistic" && input.StyleType != "stylized") ||
		!validPresetSubtype(input.StyleSubtype) || (input.StyleType == "realistic" && input.StyleSubtype != "") ||
		(input.After != nil && (input.After.ID == uuid.Nil || !utf8.ValidString(input.After.Name) || len(input.After.Name) > 4096)) {
		return StylePresetPage{}, ErrInvalidStylePresetList
	}
	if input.Limit == 0 {
		input.Limit = 50
	}
	page, err := q.store.ListStylePresetSummaries(ctx, actor, input)
	if err != nil {
		return StylePresetPage{}, fmt.Errorf("list organization style presets: %w", err)
	}
	if page.Items == nil {
		page.Items = []StylePresetSummary{}
	}
	if len(page.Items) > input.Limit {
		return StylePresetPage{}, ErrInvalidStylePresetList
	}
	for _, item := range page.Items {
		if item.ID == uuid.Nil || !utf8.ValidString(item.Name) || len(item.Name) > 4096 ||
			(item.StyleType != "realistic" && item.StyleType != "stylized") ||
			!validPresetSubtype(item.StyleSubtype) ||
			(item.StyleType == "realistic" && item.StyleSubtype != "") ||
			(item.StyleType == "stylized" && item.StyleSubtype == "") ||
			(input.StyleType != "" && item.StyleType != input.StyleType) ||
			(input.StyleSubtype != "" && item.StyleSubtype != input.StyleSubtype) {
			return StylePresetPage{}, ErrInvalidStylePresetList
		}
	}
	if page.Next != nil {
		if len(page.Items) == 0 {
			return StylePresetPage{}, ErrInvalidStylePresetList
		}
		last := page.Items[len(page.Items)-1]
		if page.Next.ID != last.ID || page.Next.Name != last.Name {
			return StylePresetPage{}, ErrInvalidStylePresetList
		}
	}
	return page, nil
}

func validPresetSubtype(value string) bool {
	switch value {
	case "", "anime_jp", "guofeng_xianxia", "cartoon_3d", "manhwa":
		return true
	default:
		return false
	}
}

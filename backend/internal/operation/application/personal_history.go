package application

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// PersonalHistoryInput selects only the caller's proven generation requests in
// their current space. Creation dates form a half-open interval [from,before).
type PersonalHistoryInput struct {
	Kind            string
	Capability      string
	Search          string
	CreatedFrom     *time.Time
	CreatedBefore   *time.Time
	Order           string
	Limit           int
	After           *TaskCursor
	FavoriteOnly    bool
	ProcessTypes    []string
	Resolutions     []string
	Ratios          []string
	GenerationModes []string
}

// Normalize sets canonical defaults before cursor binding and rejects malformed filters.
func (i *PersonalHistoryInput) Normalize() error {
	i.Search = strings.TrimSpace(i.Search)
	for _, filter := range []struct {
		values  *[]string
		allowed []string
	}{
		{&i.ProcessTypes, []string{"super_resolution", "frame_interpolation"}},
		{&i.Resolutions, []string{"1K", "2K", "4K", "8K", "720P", "1080P"}},
		{&i.GenerationModes, []string{"all_reference", "first_last_frame", "multi_frame", "lip_sync", "motion_mimic", "song", "instrumental", "tts"}},
	} {
		if err := normalizeHistoryValues(filter.values, filter.allowed); err != nil {
			return err
		}
	}
	if err := normalizeHistoryValues(&i.Ratios, nil); err != nil {
		return err
	}
	for _, ratio := range i.Ratios {
		parts := strings.Split(ratio, ":")
		if len(parts) != 2 {
			return ErrInvalidPublicQuery
		}
		for _, part := range parts {
			n, err := strconv.Atoi(part)
			if err != nil || n < 1 || n > 9999 || strconv.Itoa(n) != part {
				return ErrInvalidPublicQuery
			}
		}
	}
	if i.Order == "" {
		i.Order = "desc"
	}
	if i.Limit == 0 {
		i.Limit = 40
	}
	switch i.Kind {
	case "", "image", "video", "audio", "document":
	default:
		return ErrInvalidPublicQuery
	}
	if i.Limit < 1 || i.Limit > 200 || (i.Order != "asc" && i.Order != "desc") ||
		!utf8.ValidString(i.Search) || utf8.RuneCountInString(i.Search) > 200 ||
		!utf8.ValidString(i.Capability) || len(i.Capability) > 128 ||
		(i.After != nil && (i.After.ID == uuid.Nil || i.After.CreateTime.IsZero())) {
		return ErrInvalidPublicQuery
	}
	for _, text := range []string{i.Search, i.Capability} {
		for _, r := range text {
			if unicode.IsControl(r) {
				return ErrInvalidPublicQuery
			}
		}
	}
	for _, boundary := range []*time.Time{i.CreatedFrom, i.CreatedBefore} {
		if boundary != nil && (boundary.IsZero() || boundary.Year() < 1 || boundary.Year() > 9999) {
			return ErrInvalidPublicQuery
		}
	}
	if i.CreatedFrom != nil {
		t := i.CreatedFrom.UTC()
		i.CreatedFrom = &t
	}
	if i.CreatedBefore != nil {
		t := i.CreatedBefore.UTC()
		i.CreatedBefore = &t
	}
	if i.CreatedFrom != nil && i.CreatedBefore != nil && !i.CreatedFrom.Before(*i.CreatedBefore) {
		return ErrInvalidPublicQuery
	}
	return nil
}

// PersonalHistoryItem contains bounded browsing facts; full frozen inputs remain
// on TaskDetail and media previews retain their existing project authorization.
type PersonalHistoryItem struct {
	TaskSummary
	PromptPreview   string                  `json:"prompt_preview"`
	HistoryRevision int64                   `json:"history_revision"`
	Outputs         []PersonalHistoryOutput `json:"outputs"`
}

// PersonalHistoryPage holds a stable creation-time page, without a costly total count.
type PersonalHistoryPage struct {
	Items []PersonalHistoryItem
	Next  *TaskCursor
}

// PersonalHistoryStore proves initiation and rechecks current project access in one snapshot.
type PersonalHistoryStore interface {
	ListPersonalHistory(context.Context, identityapp.Principal, PersonalHistoryInput) (PersonalHistoryPage, error)
}

// PersonalHistoryQuery owns validation for the personal generation history.
type PersonalHistoryQuery struct{ store PersonalHistoryStore }

// NewPersonalHistoryQuery injects the scoped history reader.
func NewPersonalHistoryQuery(store PersonalHistoryStore) *PersonalHistoryQuery {
	return &PersonalHistoryQuery{store: store}
}

// List reads the caller's history without inferring authorship from the confirmer.
func (q *PersonalHistoryQuery) List(ctx context.Context, actor identityapp.Principal, input PersonalHistoryInput) (PersonalHistoryPage, error) {
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.ScopeID == uuid.Nil || actor.SessionID == uuid.Nil || actor.CredentialEpoch < 1 {
		return PersonalHistoryPage{}, identityapp.ErrForbidden
	}
	if q == nil || q.store == nil {
		return PersonalHistoryPage{}, ErrInvalidPublicQuery
	}
	if err := input.Normalize(); err != nil {
		return PersonalHistoryPage{}, err
	}
	page, err := q.store.ListPersonalHistory(ctx, actor, input)
	if err != nil {
		return PersonalHistoryPage{}, fmt.Errorf("list personal operation history: %w", err)
	}
	if len(page.Items) > input.Limit {
		return PersonalHistoryPage{}, ErrInvalidPublicQuery
	}
	if page.Items == nil {
		page.Items = []PersonalHistoryItem{}
	}
	for n := range page.Items {
		item := &page.Items[n]
		if item.ID == uuid.Nil || item.ProjectID == uuid.Nil || item.CreateTime.IsZero() || utf8.RuneCountInString(item.PromptPreview) > 240 {
			return PersonalHistoryPage{}, ErrInvalidPublicQuery
		}
		if item.Outputs == nil {
			item.Outputs = []PersonalHistoryOutput{}
		}
	}
	if page.Next != nil {
		if len(page.Items) == 0 {
			return PersonalHistoryPage{}, ErrInvalidPublicQuery
		}
		last := page.Items[len(page.Items)-1]
		if page.Next.ID != last.ID || !page.Next.CreateTime.Equal(last.CreateTime) {
			return PersonalHistoryPage{}, ErrInvalidPublicQuery
		}
	}
	return page, nil
}

func normalizeHistoryValues(values *[]string, allowed []string) error {
	*values = slices.Clone(*values)
	if len(*values) > 24 {
		return ErrInvalidPublicQuery
	}
	seen := map[string]bool{}
	for _, value := range *values {
		if value == "" || seen[value] || (allowed != nil && !slices.Contains(allowed, value)) {
			return ErrInvalidPublicQuery
		}
		seen[value] = true
	}
	slices.Sort(*values)
	return nil
}

// PersonalHistoryOutput exposes every actual candidate and personal metadata.
// Unknown physical or generation facts remain null/empty instead of being inferred.
type PersonalHistoryOutput struct {
	TaskOutput
	Width            *int     `json:"width"`
	Height           *int     `json:"height"`
	DurationMS       *int64   `json:"duration_ms"`
	Favorite         bool     `json:"favorite"`
	MetadataRevision int64    `json:"metadata_revision"`
	Resolution       string   `json:"resolution"`
	Ratio            string   `json:"ratio"`
	GenerationMode   string   `json:"generation_mode"`
	ProcessTypes     []string `json:"process_types" gorm:"-"`
	CanPublish       bool     `json:"can_publish"`
	CanDownload      bool     `json:"can_download"`
}

// HistoryPublicationAccess reads current scope write eligibility without upgrading
// project read locks. Publication still reauthorizes the exact project on commit.
type HistoryPublicationAccess interface {
	CanPublishHistory(context.Context, identityapp.Principal) (bool, error)
}

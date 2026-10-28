package application

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/domain"
)

// Command errors distinguish missing, stale and unproved dependencies.
var (
	ErrNotFound    = errors.New("subject not found")
	ErrConflict    = errors.New("subject revision conflict")
	ErrKeyConflict = errors.New("subject command key conflict")
	ErrUnavailable = errors.New("subject dependency unavailable")
)

// CommandResult is the small permanent result of one accepted command.
type CommandResult struct {
	Detail *Detail      `json:"detail,omitempty"`
	Item   *ItemResult  `json:"item,omitempty"`
	Batch  *BatchResult `json:"batch,omitempty"`
}

// StoredReceipt binds an immutable request digest to its original outcome.
type StoredReceipt struct {
	Hash   string
	Result CommandResult
}

// Transaction holds scope, media, head and receipt locks until commit.
type Transaction interface {
	Scope() ScopeFacts
	Media() MediaAccess
	Replay(context.Context, uuid.UUID, string) (*CommandResult, error)
	Find(context.Context, uuid.UUID) (domain.Head, domain.Version, bool, error)
	Publish(context.Context, domain.Head, domain.Version, int64) error
	Delete(context.Context, domain.Head, int64) error
	SetFavorite(context.Context, uuid.UUID, bool) error
	Record(context.Context, uuid.UUID, string, CommandResult) error
}

// CommandStore authorizes before invoking the transaction, including replay.
type CommandStore interface {
	Write(context.Context, identity.Principal, func(Transaction) (CommandResult, error)) (CommandResult, error)
}

// Service publishes complete settings through the owning transaction.
type Service struct {
	store CommandStore
	now   func() time.Time
}

// NewService injects the atomic repository and UTC publication clock.
func NewService(store CommandStore, now func() time.Time) *Service {
	return &Service{store: store, now: now}
}

func commandHash(action string, id uuid.UUID, body any) (string, error) {
	return domain.Digest(struct {
		Action string
		ID     uuid.UUID
		Body   any
	}{action, id, body})
}

func (s *Service) write(ctx context.Context, a identity.Principal, key uuid.UUID, hash string, run func(Transaction) (CommandResult, error)) (CommandResult, error) {
	if s == nil || s.store == nil || s.now == nil {
		return CommandResult{}, ErrUnavailable
	}
	if key == uuid.Nil {
		return CommandResult{}, domain.ErrInvalid
	}
	return s.store.Write(ctx, a, func(tx Transaction) (CommandResult, error) {
		facts := tx.Scope()
		if facts.ScopeID != a.ScopeID || facts.PhysicalScopeID != a.OrgID || facts.ScopeID == uuid.Nil || facts.PhysicalScopeID == uuid.Nil {
			return CommandResult{}, ErrNotFound
		}
		prior, err := tx.Replay(ctx, key, hash)
		if err != nil {
			return CommandResult{}, err
		}
		if prior != nil {
			return *prior, nil
		}
		result, err := run(tx)
		if err != nil {
			return CommandResult{}, err
		}
		if err := tx.Record(ctx, key, hash, result); err != nil {
			return CommandResult{}, err
		}
		return result, nil
	})
}

// Save validates all references before publishing one immutable subject version.
func (s *Service) Save(ctx context.Context, a identity.Principal, id, key uuid.UUID, in SaveInput) (Detail, error) {
	hash, err := commandHash("save", id, in)
	if err != nil {
		return Detail{}, err
	}
	if in.ExpectedRevision < 0 || id == uuid.Nil && in.ExpectedRevision != 0 || id != uuid.Nil && in.ExpectedRevision < 1 || len(in.References) > domain.MaxReferences {
		return Detail{}, domain.ErrInvalid
	}
	result, err := s.write(ctx, a, key, hash, func(tx Transaction) (CommandResult, error) {
		now := s.now().UTC().Truncate(time.Microsecond)
		h := domain.Head{ID: uuid.New(), OwnerScopeID: a.ScopeID, PhysicalScopeID: a.OrgID, Revision: 1, CreatedBy: a.ID, CreatedAt: now, UpdatedAt: now}
		v := domain.Version{ID: uuid.New(), SubjectID: h.ID, OwnerScopeID: a.ScopeID, PhysicalScopeID: a.OrgID, Number: 1, CreatedBy: a.ID, CreatedAt: now}
		favorite := false
		if id != uuid.Nil {
			old, prior, fav, err := tx.Find(ctx, id)
			if err != nil {
				return CommandResult{}, err
			}
			if old.Revision != in.ExpectedRevision || old.Revision >= math.MaxInt32 {
				return CommandResult{}, ErrConflict
			}
			h = old
			h.Revision++
			h.UpdatedAt = now
			favorite = fav
			v.SubjectID = id
			v.Number = prior.Number + 1
			v.PreviousID = &prior.ID
		}
		content := domain.Content{SchemaVersion: 1, Name: strings.TrimSpace(in.Name), Description: in.Description, References: []domain.Binding{}}
		var err error
		content.Primary, err = reference(ctx, tx, a, in.Primary)
		if err != nil {
			return CommandResult{}, err
		}
		for _, selection := range in.References {
			binding, err := reference(ctx, tx, a, selection)
			if err != nil {
				return CommandResult{}, err
			}
			content.References = append(content.References, binding)
		}
		if in.Voice != nil {
			binding, err := reference(ctx, tx, a, MediaSelection{AssetID: in.Voice.AssetID, ProjectID: in.Voice.ProjectID, BindingID: in.Voice.BindingID})
			if err != nil {
				return CommandResult{}, err
			}
			content.Voice = &domain.Voice{Name: strings.TrimSpace(in.Voice.Name), Binding: binding}
		}
		if err := content.Validate(); err != nil {
			return CommandResult{}, err
		}
		v.Content = content
		v.SHA256, err = domain.Digest(content)
		if err != nil {
			return CommandResult{}, err
		}
		h.VersionID = v.ID
		if err := tx.Publish(ctx, h, v, in.ExpectedRevision); err != nil {
			return CommandResult{}, err
		}
		detail := Present(h, v, favorite)
		return CommandResult{Detail: &detail}, nil
	})
	if err != nil {
		return Detail{}, err
	}
	if result.Detail == nil {
		return Detail{}, ErrUnavailable
	}
	return *result.Detail, nil
}

func reference(ctx context.Context, tx Transaction, a identity.Principal, selection MediaSelection) (domain.Binding, error) {
	if selection.AssetID == uuid.Nil || selection.ProjectID != nil && *selection.ProjectID == uuid.Nil || selection.BindingID != nil && *selection.BindingID == uuid.Nil {
		return domain.Binding{}, domain.ErrInvalid
	}
	if tx.Media() == nil {
		return domain.Binding{}, ErrUnavailable
	}
	fact, err := tx.Media().Reference(ctx, a, selection)
	if err != nil {
		return domain.Binding{}, err
	}
	if fact.AssetID != selection.AssetID || fact.Validate() != nil || selection.BindingID == nil && ((selection.ProjectID == nil) != (fact.SourceProjectID == nil) || selection.ProjectID != nil && *selection.ProjectID != *fact.SourceProjectID) {
		return domain.Binding{}, ErrUnavailable
	}
	return domain.Binding{ID: uuid.New(), Media: fact}, nil
}

// Delete removes a subject from future use while retaining immutable versions.
func (s *Service) Delete(ctx context.Context, a identity.Principal, id, key uuid.UUID, in RevisionInput) (ItemResult, error) {
	hash, err := commandHash("delete", id, in)
	if err != nil {
		return ItemResult{}, err
	}
	result, err := s.write(ctx, a, key, hash, func(tx Transaction) (CommandResult, error) {
		r, err := s.changeItem(ctx, tx, id, in.ExpectedRevision, "delete")
		return CommandResult{Item: &r}, err
	})
	if err != nil {
		return ItemResult{}, err
	}
	if result.Item == nil {
		return ItemResult{}, ErrUnavailable
	}
	return *result.Item, nil
}

// Favorite records an actor preference without changing shared content history.
func (s *Service) Favorite(ctx context.Context, a identity.Principal, id, key uuid.UUID, in FavoriteInput) (ItemResult, error) {
	hash, err := commandHash("favorite", id, in)
	if err != nil {
		return ItemResult{}, err
	}
	action := "unfavorite"
	if in.Favorite {
		action = "favorite"
	}
	result, err := s.write(ctx, a, key, hash, func(tx Transaction) (CommandResult, error) {
		r, err := s.changeItem(ctx, tx, id, in.ExpectedRevision, action)
		return CommandResult{Item: &r}, err
	})
	if err != nil {
		return ItemResult{}, err
	}
	if result.Item == nil {
		return ItemResult{}, ErrUnavailable
	}
	return *result.Item, nil
}

func (s *Service) changeItem(ctx context.Context, tx Transaction, id uuid.UUID, expected int64, action string) (ItemResult, error) {
	if id == uuid.Nil || expected < 1 {
		return ItemResult{}, domain.ErrInvalid
	}
	h, _, _, err := tx.Find(ctx, id)
	if err != nil {
		return ItemResult{}, err
	}
	if h.Revision != expected || h.Revision >= math.MaxInt32 {
		return ItemResult{}, ErrConflict
	}
	result := ItemResult{ID: id, Status: "succeeded", Revision: h.Revision}
	if action == "delete" {
		now := s.now().UTC().Truncate(time.Microsecond)
		h.Revision++
		h.UpdatedAt = now
		h.DeletedAt = &now
		result.Revision = h.Revision
		err = tx.Delete(ctx, h, expected)
	} else {
		favorite := action == "favorite"
		result.Favorite = &favorite
		err = tx.SetFavorite(ctx, id, favorite)
	}
	return result, err
}

// Batch commits expected per-item conflicts as explicit results under one key.
func (s *Service) Batch(ctx context.Context, a identity.Principal, key uuid.UUID, in BatchInput) (BatchResult, error) {
	if in.Action != "delete" && in.Action != "favorite" && in.Action != "unfavorite" || len(in.Items) < 1 || len(in.Items) > 120 {
		return BatchResult{}, domain.ErrInvalid
	}
	seen := map[uuid.UUID]bool{}
	for _, item := range in.Items {
		if item.ID == uuid.Nil || item.ExpectedRevision < 1 || seen[item.ID] {
			return BatchResult{}, domain.ErrInvalid
		}
		seen[item.ID] = true
	}
	hash, err := commandHash("batch", uuid.Nil, in)
	if err != nil {
		return BatchResult{}, err
	}
	result, err := s.write(ctx, a, key, hash, func(tx Transaction) (CommandResult, error) {
		batch := BatchResult{Items: make([]ItemResult, 0, len(in.Items))}
		for _, item := range in.Items {
			r, err := s.changeItem(ctx, tx, item.ID, item.ExpectedRevision, in.Action)
			if err != nil {
				code := ""
				switch {
				case errors.Is(err, ErrNotFound):
					code = "not_found"
				case errors.Is(err, ErrConflict):
					code = "revision_conflict"
				default:
					return CommandResult{}, err
				}
				r = ItemResult{ID: item.ID, Status: "failed", Code: code}
			}
			batch.Items = append(batch.Items, r)
		}
		return CommandResult{Batch: &batch}, nil
	})
	if err != nil {
		return BatchResult{}, err
	}
	if result.Batch == nil {
		return BatchResult{}, ErrUnavailable
	}
	return *result.Batch, nil
}

func presentMedia(binding domain.Binding) Media {
	f := binding.Media
	return Media{BindingID: binding.ID, AssetID: f.AssetID, AssetRevision: f.Revision, Kind: f.Kind, MIMEType: f.MIMEType, FileName: f.FileName, DurationMS: f.DurationMS, Width: f.Width, Height: f.Height}
}

// Present excludes private source ownership and hashes from public content.
func Present(h domain.Head, v domain.Version, favorite bool) Detail {
	result := Detail{ID: h.ID, OwnerScopeID: h.OwnerScopeID, Revision: h.Revision, VersionID: v.ID, Name: v.Content.Name, Description: v.Content.Description, Primary: presentMedia(v.Content.Primary), References: []Media{}, Favorite: favorite, CreatedAt: h.CreatedAt, UpdatedAt: h.UpdatedAt}
	for _, binding := range v.Content.References {
		result.References = append(result.References, presentMedia(binding))
	}
	if voice := v.Content.Voice; voice != nil {
		result.Voice = &Voice{Name: voice.Name, Media: presentMedia(voice.Binding)}
	}
	return result
}

// Validate checks bounded filters before query execution.
func (in ListInput) Validate() error {
	if !utf8.ValidString(in.Query) || utf8.RuneCountInString(in.Query) > 512 || strings.ContainsRune(in.Query, 0) || in.Page < 1 || in.Page > 100000 || in.PageSize < 1 || in.PageSize > 120 {
		return domain.ErrInvalid
	}
	if in.TimeField != "created" && in.TimeField != "updated" {
		return domain.ErrInvalid
	}
	if in.Order != "created_asc" && in.Order != "created_desc" && in.Order != "updated_asc" && in.Order != "updated_desc" {
		return domain.ErrInvalid
	}
	if in.From != nil && in.To != nil && !in.To.After(*in.From) {
		return domain.ErrInvalid
	}
	return nil
}

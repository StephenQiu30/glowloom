// Package application coordinates authorized canvas document commands.
package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
)

var (
	// ErrNotFound hides canvases and projects outside the authorized scope.
	ErrNotFound = errors.New("canvas not found")
	// ErrIdempotencyConflict means a durable key was reused with another request.
	ErrIdempotencyConflict = errors.New("idempotency conflict")
)

// RevisionConflict includes the current document revision for safe UI recovery.
type RevisionConflict struct{ CurrentRevision int64 }

func (e *RevisionConflict) Error() string { return "canvas revision conflict" }

// CreateInput creates a project-scoped canvas with no business scope binding.
type CreateInput struct {
	Name  string          `json:"name"`
	Scope json.RawMessage `json:"scope" swaggertype:"object"`
}

// CommandsInput applies a single optimistic document transaction.
type CommandsInput struct {
	ExpectedRevision int64            `json:"expected_revision"`
	Commands         []domain.Command `json:"commands"`
}

// CommandResult marks one atomically committed command.
type CommandResult struct {
	OK bool `json:"ok"`
}

// Result includes the authoritative document snapshot returned by commands.
type Result struct {
	domain.Document
	Results []CommandResult `json:"results"`
}

// RenameInput renames a document at its current revision.
type RenameInput struct {
	ExpectedRevision int64  `json:"expected_revision"`
	Name             string `json:"name"`
}

// DeleteInput soft-deletes a canvas without deleting referenced media.
type DeleteInput struct {
	ExpectedRevision int64 `json:"expected_revision"`
}

// DeleteResult is the durable idempotent deletion result.
type DeleteResult struct {
	ID       uuid.UUID `json:"id"`
	Revision int64     `json:"revision"`
	Deleted  bool      `json:"deleted"`
}

// MediaReader is the small application boundary needed for new media references.
type MediaReader interface {
	Reference(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (mediaapp.AssetSummary, error)
}

// Store is the transaction boundary that rechecks project access and persists replay results.
type Store interface {
	List(context.Context, identityapp.Principal, uuid.UUID) ([]domain.Document, error)
	Get(context.Context, identityapp.Principal, uuid.UUID) (domain.Document, error)
	Create(context.Context, identityapp.Principal, uuid.UUID, string, CreateInput) (domain.Document, error)
	Execute(context.Context, identityapp.Principal, uuid.UUID, string, CommandsInput) (Result, error)
	Rename(context.Context, identityapp.Principal, uuid.UUID, string, RenameInput) (domain.Document, error)
	Delete(context.Context, identityapp.Principal, uuid.UUID, string, DeleteInput) (DeleteResult, error)
}

// Service validates transport-independent input before entering persistence.
type Service struct{ store Store }

// NewService injects the project-scoped document store.
func NewService(store Store) *Service { return &Service{store: store} }

// List returns visible documents in a project.
func (s *Service) List(ctx context.Context, a identityapp.Principal, p uuid.UUID) ([]domain.Document, error) {
	return s.store.List(ctx, a, p)
}

// Get returns a visible document and its live graph.
func (s *Service) Get(ctx context.Context, a identityapp.Principal, id uuid.UUID) (domain.Document, error) {
	return s.store.Get(ctx, a, id)
}

// Create validates project scope and a bounded display name.
func (s *Service) Create(ctx context.Context, a identityapp.Principal, p uuid.UUID, key string, input CreateInput) (domain.Document, error) {
	input.Name = strings.TrimSpace(input.Name)
	var scope map[string]any
	if len(input.Scope) == 0 {
		input.Scope = json.RawMessage(`{}`)
	}
	if !validKey(key) || input.Name == "" || !utf8.ValidString(input.Name) || utf8.RuneCountInString(input.Name) > 128 || json.Unmarshal(input.Scope, &scope) != nil || scope == nil || len(scope) != 0 {
		return domain.Document{}, domain.ErrInvalidCommand
	}
	return s.store.Create(ctx, a, p, key, input)
}

// Execute validates the revision and batch bounds; the store applies it atomically.
func (s *Service) Execute(ctx context.Context, a identityapp.Principal, id uuid.UUID, key string, input CommandsInput) (Result, error) {
	if !validKey(key) || input.ExpectedRevision < 1 || len(input.Commands) < 1 || len(input.Commands) > 100 {
		return Result{}, domain.ErrInvalidCommand
	}
	return s.store.Execute(ctx, a, id, key, input)
}
func validKey(key string) bool { id, err := uuid.Parse(key); return err == nil && id != uuid.Nil }

// Rename validates bounded user-facing metadata before the optimistic transaction.
func (s *Service) Rename(ctx context.Context, a identityapp.Principal, id uuid.UUID, key string, input RenameInput) (domain.Document, error) {
	input.Name = strings.TrimSpace(input.Name)
	if !validKey(key) || input.ExpectedRevision < 1 || input.Name == "" || !utf8.ValidString(input.Name) || utf8.RuneCountInString(input.Name) > 128 {
		return domain.Document{}, domain.ErrInvalidCommand
	}
	return s.store.Rename(ctx, a, id, key, input)
}

// Delete removes only the current canvas; media remains owned by its original context.
func (s *Service) Delete(ctx context.Context, a identityapp.Principal, id uuid.UUID, key string, input DeleteInput) (DeleteResult, error) {
	if !validKey(key) || input.ExpectedRevision < 1 {
		return DeleteResult{}, domain.ErrInvalidCommand
	}
	return s.store.Delete(ctx, a, id, key, input)
}

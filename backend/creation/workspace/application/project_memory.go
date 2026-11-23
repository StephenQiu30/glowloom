package application

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
)

// Project memory errors describe failures that callers can correct.
var (
	ErrInvalidProjectMemory = errors.New("invalid project memory")
	ErrProjectMemoryLimit   = errors.New("project memory limit reached")
)

const MaxActiveProjectMemories = 24

// ProjectMemory is an immutable, explicitly published project decision revision.
type ProjectMemory struct {
	ID          uuid.UUID `json:"id"`
	ProjectID   uuid.UUID `json:"project_id"`
	Key         string    `json:"key"`
	Revision    int64     `json:"revision"`
	Statement   string    `json:"statement"`
	Withdrawn   bool      `json:"withdrawn"`
	ConfirmedBy uuid.UUID `json:"confirmed_by"`
	ConfirmedAt time.Time `json:"confirmed_at"`
}

// ProjectMemoryChange is a user's publication, correction, or withdrawal.
type ProjectMemoryChange struct {
	ExpectedRevision *int64 `json:"expected_revision" binding:"required"`
	Statement        string `json:"statement" maxLength:"512"`
	Withdraw         bool   `json:"withdraw"`
}

// ProjectMemoryView contains the visible active decisions and edit authority.
type ProjectMemoryView struct {
	Items   []ProjectMemory `json:"items"`
	CanEdit bool            `json:"can_edit"`
}

// ProjectMemoryStore enforces current authority with the read or write.
type ProjectMemoryStore interface {
	ListProjectMemories(context.Context, identity.Principal, uuid.UUID) (ProjectMemoryView, error)
	ProjectMemoryHistory(context.Context, identity.Principal, uuid.UUID, string, int64) ([]ProjectMemory, error)
	ChangeProjectMemory(context.Context, identity.Principal, uuid.UUID, string, uuid.UUID, ProjectMemoryChange, time.Time) (ProjectMemory, error)
}

// ProjectMemories exposes explicitly confirmed decisions to authorized project members.
type ProjectMemories struct {
	store ProjectMemoryStore
	now   func() time.Time
}

// NewProjectMemories binds project memory to the project owner.
func NewProjectMemories(store ProjectMemoryStore, now func() time.Time) *ProjectMemories {
	return &ProjectMemories{store: store, now: now}
}

// List returns current active decisions.
func (s *ProjectMemories) List(ctx context.Context, actor identity.Principal, project uuid.UUID) (ProjectMemoryView, error) {
	return s.store.ListProjectMemories(ctx, actor, project)
}

// History returns up to 30 revisions preceding the optional revision cursor.
func (s *ProjectMemories) History(ctx context.Context, actor identity.Principal, project uuid.UUID, key string, before int64) ([]ProjectMemory, error) {
	if !validMemoryKey(key) || before < 0 {
		return nil, ErrInvalidProjectMemory
	}
	return s.store.ProjectMemoryHistory(ctx, actor, project, key, before)
}

// Change applies the explicit confirmation to the expected revision.
func (s *ProjectMemories) Change(ctx context.Context, actor identity.Principal, project uuid.UUID, key string, request uuid.UUID, input ProjectMemoryChange) (ProjectMemory, error) {
	input.Statement = strings.TrimSpace(input.Statement)
	if !validMemoryKey(key) || request == uuid.Nil || input.ExpectedRevision == nil || *input.ExpectedRevision < 0 ||
		!utf8.ValidString(input.Statement) || strings.ContainsRune(input.Statement, 0) || utf8.RuneCountInString(input.Statement) > 512 ||
		(input.Withdraw && input.Statement != "") || (!input.Withdraw && input.Statement == "") {
		return ProjectMemory{}, ErrInvalidProjectMemory
	}
	return s.store.ChangeProjectMemory(ctx, actor, project, key, request, input, s.now().UTC())
}

func validMemoryKey(key string) bool {
	if key == "" || !utf8.ValidString(key) || key != strings.TrimSpace(key) || utf8.RuneCountInString(key) > 48 {
		return false
	}
	for _, r := range key {
		if !(unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

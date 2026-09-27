package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

var (
	// ErrInvalidBootstrap means the first administrator's input is incomplete.
	ErrInvalidBootstrap = errors.New("invalid administrator bootstrap")
	// ErrAlreadyBootstrapped means an account already exists in this installation.
	ErrAlreadyBootstrapped = errors.New("administrator bootstrap is already complete")
	// ErrBootstrapOrganizationConflict means the MVP organization is ambiguous or unavailable.
	ErrBootstrapOrganizationConflict = errors.New("bootstrap organization is unavailable")
)

// BootstrapAdminInput contains operator-supplied first-account details.
// InitialPassword must come from a private interactive terminal prompt.
type BootstrapAdminInput struct {
	LoginName        string
	DisplayName      string
	OrganizationName string
	InitialPassword  string
}

// BootstrapAdminStore locates the MVP organization and commits the first
// administrator with both required events under a global transaction lock.
type BootstrapAdminStore interface {
	FindBootstrapOrganization(context.Context) (uuid.UUID, error)
	BootstrapAdminWithEvents(context.Context, string, domain.User, []OutboxEvent) (domain.User, error)
}

// BootstrapAdminCommand initializes the first administrator without a default credential.
type BootstrapAdminCommand struct {
	store BootstrapAdminStore
	now   func() time.Time
}

// NewBootstrapAdminCommand injects the transactional store and clock.
func NewBootstrapAdminCommand(store BootstrapAdminStore, now func() time.Time) *BootstrapAdminCommand {
	return &BootstrapAdminCommand{store: store, now: now}
}

// Execute hashes the password before entering the organization/account transaction.
func (c *BootstrapAdminCommand) Execute(ctx context.Context, input BootstrapAdminInput) (CreatedUser, error) {
	if c == nil || c.store == nil || c.now == nil {
		return CreatedUser{}, ErrInvalidBootstrap
	}
	loginName := strings.TrimSpace(input.LoginName)
	displayName := strings.TrimSpace(input.DisplayName)
	if displayName == "" {
		displayName = loginName
	}
	orgName := strings.TrimSpace(input.OrganizationName)
	if orgName == "" {
		orgName = "Lanverse"
	}
	if loginName == "" || utf8.RuneCountInString(displayName) > 50 ||
		utf8.RuneCountInString(orgName) > 100 || input.InitialPassword == "" {
		return CreatedUser{}, ErrInvalidBootstrap
	}
	hash, err := domain.HashPassword(input.InitialPassword, "")
	if err != nil {
		return CreatedUser{}, err
	}
	orgID, err := c.store.FindBootstrapOrganization(ctx)
	if err != nil {
		return CreatedUser{}, fmt.Errorf("find bootstrap organization: %w", err)
	}
	if orgID == uuid.Nil {
		orgID = uuid.New()
	}
	user := domain.User{
		ID: uuid.New(), OrgID: orgID, LoginName: loginName, DisplayName: displayName,
		Role: domain.RoleAdmin, Status: domain.StatusActive, PasswordHash: hash,
		MustChangePassword: true, SessionEpoch: 1, Revision: 1,
	}
	events, err := bootstrapAdminEvents(user, c.now().UTC())
	if err != nil {
		return CreatedUser{}, fmt.Errorf("build bootstrap events: %w", err)
	}
	saved, err := c.store.BootstrapAdminWithEvents(ctx, orgName, user, events)
	if err != nil {
		return CreatedUser{}, fmt.Errorf("bootstrap first administrator: %w", err)
	}
	return CreatedUser{
		ID: saved.ID, OrgID: saved.OrgID, LoginName: saved.LoginName,
		DisplayName: saved.DisplayName, Role: saved.Role, Status: saved.Status,
		MustChangePassword: saved.MustChangePassword, Revision: saved.Revision,
		CreateTime: saved.CreateTime,
	}, nil
}

func bootstrapAdminEvents(user domain.User, occurredAt time.Time) ([]OutboxEvent, error) {
	changedID, auditID := uuid.New(), uuid.New()
	changedPayload, err := json.Marshal(map[string]any{
		"event_id": changedID, "event_type": userChangedTopic,
		"occurred_at": occurredAt, "org_id": user.OrgID,
		"actor":     map[string]any{"kind": "system"},
		"aggregate": map[string]any{"type": "user", "id": user.ID, "revision": user.Revision},
		"data":      map[string]any{"change": "created"},
	})
	if err != nil {
		return nil, fmt.Errorf("encode bootstrap account event: %w", err)
	}
	auditPayload, err := json.Marshal(map[string]any{
		"event_id": auditID, "event_type": auditTopic,
		"occurred_at": occurredAt, "org_id": user.OrgID,
		"actor":     map[string]any{"kind": "system"},
		"aggregate": map[string]any{"type": "audit", "id": auditID},
		"data": map[string]any{
			"action": "user.created", "object": map[string]any{"type": "user", "id": user.ID},
			"after": map[string]any{
				"role": user.Role, "status": user.Status,
				"must_change_password": user.MustChangePassword,
			},
			"request_id": "bootstrap-" + uuid.NewString(),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("encode bootstrap audit event: %w", err)
	}
	key := user.OrgID.String()
	return []OutboxEvent{
		{ID: changedID, Topic: userChangedTopic, PartitionKey: key, Payload: changedPayload},
		{ID: auditID, Topic: auditTopic, PartitionKey: key, Payload: auditPayload},
	}, nil
}

package identity_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
)

type bootstrapStoreStub struct {
	orgID      uuid.UUID
	orgName    string
	user       domain.User
	events     []identityapp.OutboxEvent
	findErr    error
	createErr  error
	createCall bool
}

func (s *bootstrapStoreStub) FindBootstrapOrganization(context.Context) (uuid.UUID, error) {
	return s.orgID, s.findErr
}

func (s *bootstrapStoreStub) BootstrapAdminWithEvents(_ context.Context, orgName string, user domain.User, events []identityapp.OutboxEvent) (domain.User, error) {
	s.createCall, s.orgName, s.user, s.events = true, orgName, user, events
	if s.createErr != nil {
		return domain.User{}, s.createErr
	}
	user.CreateTime = time.Now().UTC()
	return user, nil
}

func TestBootstrapAdminCreatesFirstAccountAndSystemAudit(t *testing.T) {
	store := &bootstrapStoreStub{}
	command := identityapp.NewBootstrapAdminCommand(store, time.Now)
	result, err := command.Execute(t.Context(), identityapp.BootstrapAdminInput{
		LoginName: "first-admin", DisplayName: "First Admin", OrganizationName: "Lanverse",
		InitialPassword: "initialPassword123",
	})
	if err != nil || !store.createCall || store.orgName != "Lanverse" ||
		store.user.ID == uuid.Nil || store.user.OrgID == uuid.Nil ||
		store.user.Role != domain.RoleAdmin || store.user.Status != domain.StatusActive ||
		!store.user.MustChangePassword || store.user.SessionEpoch != 1 || store.user.Revision != 1 ||
		!domain.VerifyPassword(store.user.PasswordHash, "initialPassword123") ||
		result.ID != store.user.ID || result.OrgID != store.user.OrgID || len(store.events) != 2 {
		t.Fatalf("bootstrap IDs = %s/%s, role %s, revision %d, error %v",
			result.ID, store.user.ID, store.user.Role, store.user.Revision, err)
	}
	for _, event := range store.events {
		if event.PartitionKey != result.OrgID.String() ||
			strings.Contains(string(event.Payload), "initialPassword123") ||
			strings.Contains(string(event.Payload), store.user.PasswordHash) {
			t.Fatal("bootstrap event has wrong routing or exposed password data")
		}
		if event.Topic == "lanverse.audit.recorded.v1" {
			record, err := auditapp.NewIdentityActionParser().Parse(inbox.Record{
				Topic: event.Topic, Key: []byte(event.PartitionKey), Value: event.Payload,
			})
			if err != nil || record.Action != "user.created" || record.ActorKind != "system" ||
				record.ActorID != nil || record.ObjectID != result.ID.String() {
				t.Fatalf("bootstrap audit = %+v, error %v", record, err)
			}
		}
	}
}

func TestBootstrapAdminReusesExistingOrganization(t *testing.T) {
	orgID := uuid.New()
	store := &bootstrapStoreStub{orgID: orgID}
	command := identityapp.NewBootstrapAdminCommand(store, time.Now)
	result, err := command.Execute(t.Context(), identityapp.BootstrapAdminInput{
		LoginName: "admin", InitialPassword: "initialPassword123",
	})
	if err != nil || result.OrgID != orgID || store.user.OrgID != orgID ||
		store.user.DisplayName != "admin" || store.orgName != "Lanverse" {
		t.Fatalf("existing organization bootstrap = %s, stored org %s, org name %q, error %v",
			result.OrgID, store.user.OrgID, store.orgName, err)
	}
}

func TestBootstrapAdminRejectsInvalidInputAndPreservesStoreError(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input identityapp.BootstrapAdminInput
		want  error
	}{
		{"missing login", identityapp.BootstrapAdminInput{InitialPassword: "initialPassword123"}, identityapp.ErrInvalidBootstrap},
		{"weak password", identityapp.BootstrapAdminInput{LoginName: "admin", InitialPassword: "short"}, domain.ErrWeakPassword},
		{"long display name", identityapp.BootstrapAdminInput{LoginName: "admin", DisplayName: strings.Repeat("x", 51), InitialPassword: "initialPassword123"}, identityapp.ErrInvalidBootstrap},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &bootstrapStoreStub{}
			_, err := identityapp.NewBootstrapAdminCommand(store, time.Now).Execute(t.Context(), tc.input)
			if !errors.Is(err, tc.want) || store.createCall {
				t.Fatalf("invalid bootstrap error = %v, store called %t", err, store.createCall)
			}
		})
	}
	sentinel := errors.New("database unavailable")
	store := &bootstrapStoreStub{createErr: sentinel}
	_, err := identityapp.NewBootstrapAdminCommand(store, time.Now).Execute(t.Context(), identityapp.BootstrapAdminInput{
		LoginName: "admin", InitialPassword: "initialPassword123",
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("bootstrap store error = %v", err)
	}
}

package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/credentialproof"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
)

func existingIdentityTransaction(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("LV_TEST_EXISTING_DB_DSN")
	if dsn == "" {
		t.Skip("existing database connection absent; integration not verified")
	}
	// testing cancels T.Context before Cleanup; keep the transaction alive until
	// our explicit rollback, with a separate bounded deadline and cleanup.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 20*time.Second)
	t.Cleanup(cancel)
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal("connect existing database failed")
	}
	t.Cleanup(func() { _ = conn.Close() })
	tx := conn.DB.WithContext(ctx).Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() {
		if err := tx.Rollback().Error; err != nil {
			t.Error("rollback existing database test failed")
		}
	})
	if err := tx.Exec("SET LOCAL lock_timeout='1s'; SET LOCAL statement_timeout='10s'").Error; err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../db/migrations/001_user_identity_expand.sql")
	if err != nil {
		t.Fatal(err)
	}
	var installed bool
	if err := tx.Raw("SELECT to_regclass('identity.auth_challenge') IS NOT NULL").Scan(&installed).Error; err != nil {
		t.Fatal(err)
	}
	if !installed {
		if err := tx.Exec(string(migration)).Error; err != nil {
			t.Fatal(err)
		}
	}
	governanceMigration, err := os.ReadFile("../../db/migrations/006_identity_governance.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(string(governanceMigration)).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec("UPDATE identity.platform_settings SET registration_enabled=true").Error; err != nil {
		t.Fatal(err)
	}
	return tx
}

func TestEmailSessionRecovery(t *testing.T) {
	tx := existingIdentityTransaction(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	protector, err := credentialproof.New("test-only", []byte(strings.Repeat("h", 32)), []byte(strings.Repeat("e", 32)))
	if err != nil {
		t.Fatal(err)
	}
	store := pgidentity.NewStore(tx)
	auth := identity.NewEmailAuth(store, protector, identity.EmailPolicy{TermsVersion: "test-terms", SourceHourlyLimit: 100, GlobalHourlyLimit: 1000}, func() time.Time { return now })
	id := uuid.New()
	hash, err := domain.HashPassword("old long password", "")
	if err != nil {
		t.Fatal(err)
	}
	err = store.EmailTransaction(t.Context(), func(t identity.EmailTx) error {
		return t.CreatePersonalAccount(domain.User{ID: id, OrgID: uuid.New(), LoginName: id.String(), Email: "recover@identity.invalid", EmailVerifiedAt: now, EmailVersion: 1, DisplayName: "创作者", PasswordHash: hash, PasswordChangedAt: now, TermsVersion: "test-terms", TermsAcceptedAt: now})
	})
	if err != nil {
		t.Fatal(err)
	}
	login, err := auth.Login(t.Context(), " RECOVER@identity.invalid ", "old long password", false)
	if err != nil {
		t.Fatal(err)
	}
	if login.CSRFToken == "" || len(login.Session.CSRFHash) != 32 {
		t.Fatal("session missing CSRF binding")
	}
	if err := auth.Reauthenticate(t.Context(), login.Token, "wrong"); !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Fatal("wrong reauthentication password accepted")
	}
	if err := auth.Reauthenticate(t.Context(), login.Token, "old long password"); err != nil {
		t.Fatal(err)
	}
	current, err := auth.Current(t.Context(), login.Token)
	if err != nil || !current.Session.RecentlyAuthenticated(current.User, now) {
		t.Fatal("recent authentication missing")
	}
	if current.CSRFToken != login.CSRFToken || current.Session.RecentlyAuthenticated(current.User, now.Add(5*time.Minute)) {
		t.Fatal("CSRF or reauthentication boundary mismatch")
	}
	if _, err := auth.StartPasswordReset(t.Context(), "recover@identity.invalid", "fixture-source"); err != nil {
		t.Fatal(err)
	}
	var mail struct {
		EventID          uuid.UUID
		KeyID            string
		EncryptedPayload []byte
	}
	if err := tx.Raw(`SELECT event_id,key_id,encrypted_payload FROM identity.mail_delivery WHERE template='password_reset' AND recipient='recover@identity.invalid' ORDER BY created_at DESC LIMIT 1`).Scan(&mail).Error; err != nil {
		t.Fatal(err)
	}
	plain, err := protector.Open(mail.EventID, mail.KeyID, mail.EncryptedPayload)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(plain, &payload); err != nil {
		t.Fatal(err)
	}
	now = now.Add(30 * time.Minute)
	if err := auth.CompletePasswordReset(t.Context(), payload.Token, "new8Pass", "new8Pass"); !errors.Is(err, identity.ErrProofInvalid) {
		t.Fatalf("exact expiry = %v", err)
	}
	now = now.Add(-time.Nanosecond)
	if err := auth.CompletePasswordReset(t.Context(), payload.Token, "new8Pass", "new8Pass"); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Current(t.Context(), login.Token); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatal("old session survived password reset")
	}
	if err := auth.CompletePasswordReset(t.Context(), payload.Token, "other long password", "other long password"); !errors.Is(err, identity.ErrProofInvalid) {
		t.Fatal("reset replay accepted")
	}
	if _, err := auth.Login(t.Context(), "recover@identity.invalid", "old long password", false); !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Fatal("old password survived reset")
	}
	if _, err := auth.Login(t.Context(), "recover@identity.invalid", "new8Pass", false); err != nil {
		t.Fatal(err)
	}
	// SMTP budgets or registration policy being unavailable must not break login.
	loginOnly := identity.NewEmailAuth(store, protector, identity.EmailPolicy{}, func() time.Time { return now })
	if _, err := loginOnly.Login(t.Context(), "recover@identity.invalid", "new8Pass", false); err != nil {
		t.Fatalf("registration configuration blocked existing login: %v", err)
	}
}

func TestRegistrationReplayAndBudget(t *testing.T) {
	tx := existingIdentityTransaction(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	protector, err := credentialproof.New("test-only", []byte(strings.Repeat("h", 32)), []byte(strings.Repeat("e", 32)))
	if err != nil {
		t.Fatal(err)
	}
	auth := identity.NewEmailAuth(pgidentity.NewStore(tx), protector, identity.EmailPolicy{TermsVersion: "test-terms", SourceHourlyLimit: 100, GlobalHourlyLimit: 1000, BlockedPasswords: []string{"passwordpassword"}}, func() time.Time { return now })
	start, err := auth.StartRegistration(t.Context(), "TEST@registration.invalid", "")
	if err != nil {
		t.Fatal(err)
	}
	if start.OperationID == uuid.Nil || start.FlowSecret == "" {
		t.Fatal("flow not bound")
	}
	if _, err := auth.ResendRegistration(t.Context(), start.OperationID, start.FlowSecret); !errors.Is(err, identity.ErrRateLimited) {
		t.Fatalf("immediate resend = %v", err)
	}
	var mail struct {
		EventID          uuid.UUID
		EncryptedPayload []byte
		KeyID            string
	}
	if err := tx.Raw("SELECT event_id,encrypted_payload,key_id FROM identity.mail_delivery WHERE recipient=? ORDER BY created_at DESC LIMIT 1", "test@registration.invalid").Scan(&mail).Error; err != nil {
		t.Fatal(err)
	}
	plain, err := protector.Open(mail.EventID, mail.KeyID, mail.EncryptedPayload)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(plain, &payload); err != nil || len(payload.Code) != 6 {
		t.Fatal("verification mail not enqueued")
	}
	if _, err := auth.VerifyRegistration(t.Context(), start.OperationID, "different flow", payload.Code); !errors.Is(err, identity.ErrProofInvalid) {
		t.Fatal("flow binding bypass")
	}
	proof, err := auth.VerifyRegistration(t.Context(), start.OperationID, start.FlowSecret, payload.Code)
	if err != nil || proof.Outcome != "set_password" {
		t.Fatalf("verify: %v", err)
	}
	input := identity.CompleteRegistrationInput{OperationID: start.OperationID, FlowSecret: start.FlowSecret, Grant: proof.Grant, DisplayName: "创作者", Password: "join8Pwd", ConfirmPassword: "join8Pwd", TermsVersion: "test-terms", TermsAccepted: true}
	for _, invalid := range []identity.CompleteRegistrationInput{
		func() identity.CompleteRegistrationInput { v := input; v.TermsAccepted = false; return v }(),
		func() identity.CompleteRegistrationInput { v := input; v.TermsVersion = "older-terms"; return v }(),
	} {
		if _, err := auth.CompleteRegistration(t.Context(), invalid); !errors.Is(err, domain.ErrInvalidProfile) {
			t.Fatal("registration accepted missing or stale consent")
		}
	}
	id, err := auth.CompleteRegistration(t.Context(), input)
	if err != nil || id == uuid.Nil {
		t.Fatalf("complete: %v", err)
	}
	var consent struct {
		TermsVersion    string
		TermsAcceptedAt time.Time
	}
	if err := tx.Raw(`SELECT terms_version,terms_accepted_at FROM identity."user" WHERE id=?`, id).Scan(&consent).Error; err != nil || consent.TermsVersion != "test-terms" || !consent.TermsAcceptedAt.Equal(now) {
		t.Fatal("consent was not atomically recorded at server time")
	}
	for range 20 {
		if _, err := auth.CompleteRegistration(t.Context(), input); !errors.Is(err, identity.ErrProofInvalid) {
			t.Fatal("grant replay accepted")
		}
	}
	var count int64
	if err := tx.Raw(`SELECT count(*) FROM identity.user_session WHERE account_id=?`, id).Scan(&count).Error; err != nil || count != 0 {
		t.Fatal("registration auto logged in")
	}
	if err := tx.Raw(`SELECT count(*) FROM workspace.creation_scope WHERE owner_user_id=? AND kind='personal'`, id).Scan(&count).Error; err != nil || count != 1 {
		t.Fatal("personal scope not atomically created")
	}
	if err := tx.Raw(`SELECT count(*) FROM identity."user" WHERE email_key=?`, "test@registration.invalid").Scan(&count).Error; err != nil || count != 1 {
		t.Fatal("duplicate identities")
	}
	now = now.Add(time.Minute)
	second, err := auth.StartRegistration(t.Context(), "budget@registration.invalid", "")
	if err != nil {
		t.Fatal(err)
	}
	for range 4 {
		if _, err := auth.VerifyRegistration(t.Context(), second.OperationID, second.FlowSecret, "bad"); !errors.Is(err, identity.ErrProofInvalid) {
			t.Fatal("bad code accepted")
		}
	}
	now = now.Add(time.Minute)
	if _, err := auth.ResendRegistration(t.Context(), second.OperationID, second.FlowSecret); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.VerifyRegistration(t.Context(), second.OperationID, second.FlowSecret, "bad"); !errors.Is(err, identity.ErrProofInvalid) {
		t.Fatal("fifth failure accepted")
	}
	if _, err := auth.ResendRegistration(t.Context(), second.OperationID, second.FlowSecret); !errors.Is(err, identity.ErrRateLimited) {
		t.Fatal("resend reset failure budget")
	}
}

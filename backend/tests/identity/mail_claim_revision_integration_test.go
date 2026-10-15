package identity_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
)

func TestMailClaimRechecksInvitationAfterCleanup(t *testing.T) {
	for _, action := range []string{"resend", "revoke"} {
		t.Run(action, func(t *testing.T) {
			tx, governance, actor, now := governanceFixture(t)
			invitation, err := governance.CreateAccountInvitation(t.Context(), actor, uuid.New(), "claim-interleave@example.invalid")
			if err != nil {
				t.Fatal(err)
			}
			var original struct{ EventID uuid.UUID }
			if err := tx.Raw(`SELECT event_id FROM identity.mail_delivery WHERE invitation_id=? AND invitation_revision=1`, invitation.ID).Scan(&original).Error; err != nil {
				t.Fatal(err)
			}
			if original.EventID == uuid.Nil {
				t.Fatal("invitation mail missing")
			}
			if action == "resend" {
				if err := tx.Exec(`UPDATE identity.email_budget SET sent_at='{}' WHERE bucket_key=?`, "invitation/"+invitation.EmailKey).Error; err != nil {
					t.Fatal(err)
				}
			}

			// Real mutations run deterministically after cleanup's snapshot and before
			// the next SQL statement. The same rollback connection makes the intervening
			// revision visible without publishing fixtures for a second DB connection.
			fired := false
			callback := "test:invitation_after_cleanup_" + uuid.NewString()
			if err := tx.Callback().Raw().After("gorm:raw").Register(callback, func(statement *gorm.DB) {
				if fired || !strings.Contains(statement.Statement.SQL.String(), "FROM identity.invitation i WHERE m.invitation_id=i.id") {
					return
				}
				fired = true
				var state string
				if err := tx.Raw(`SELECT status FROM identity.mail_delivery WHERE event_id=?`, original.EventID).Scan(&state).Error; err != nil {
					_ = statement.AddError(err)
					return
				}
				if state != "pending" {
					_ = statement.AddError(fmt.Errorf("cleanup did not observe live original mail: %s", state))
					return
				}
				if action == "resend" {
					_ = statement.AddError(governance.ResendInvitation(t.Context(), actor, invitation.ID, 1, nil))
				} else {
					_ = statement.AddError(governance.RevokeInvitation(t.Context(), actor, invitation.ID, 1, "Withdraw before mail lease", nil))
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := tx.Callback().Raw().Remove(callback); err != nil {
					t.Error(err)
				}
			})

			store := pgidentity.NewMailStore(tx)
			jobs, err := store.Claim(t.Context(), 100, now)
			if err != nil {
				t.Fatal(err)
			}
			if !fired {
				t.Fatal("cleanup/lease interleaving did not execute")
			}
			currentCount := 0
			for _, job := range jobs {
				if job.EventID == original.EventID {
					t.Fatalf("%s between cleanup and lease newly claimed obsolete invitation", action)
				}
				if job.Recipient == invitation.EmailKey {
					currentCount++
				}
			}
			expected := 0
			if action == "resend" {
				expected = 1
			}
			if currentCount != expected {
				t.Fatalf("current revision lease count=%d want=%d", currentCount, expected)
			}
			// A later cleanup clears stale secrets; the invalid row never had a lease.
			if _, err := store.Claim(t.Context(), 100, now); err != nil {
				t.Fatal(err)
			}
			var terminal struct {
				Status           string
				AttemptCount     int
				EncryptedPayload []byte
			}
			if err := tx.Raw(`SELECT status,attempt_count,encrypted_payload FROM identity.mail_delivery WHERE event_id=?`, original.EventID).Scan(&terminal).Error; err != nil {
				t.Fatal(err)
			}
			if terminal.Status != "expired" || terminal.AttemptCount != 0 || len(terminal.EncryptedPayload) != 0 {
				t.Fatalf("obsolete mail status=%s attempts=%d retained_bytes=%d", terminal.Status, terminal.AttemptCount, len(terminal.EncryptedPayload))
			}
		})
	}
}

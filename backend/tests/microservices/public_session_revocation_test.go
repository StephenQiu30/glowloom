package microservices_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

func publicRevocationInput(t *testing.T, admin *gorm.DB, actor identity.Principal, action identity.SessionRevocationAction, target uuid.UUID) identity.SessionRevocationInput {
	t.Helper()
	csrf := sha256.Sum256([]byte(uuid.NewString()))
	if err := admin.Exec(`UPDATE identity.user_session SET csrf_hash=? WHERE id=?`, csrf[:], actor.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	token := sha256.Sum256([]byte("signed-access-" + actor.SessionID.String()))
	return identity.SessionRevocationInput{Key: uuid.New(), Action: action, TargetID: target, Primary: identity.SessionCredentialAccess, Proofs: []identity.SessionRevocationProof{{Kind: identity.SessionCredentialAccess, TokenDigest: token[:], CSRFHash: csrf[:]}}}
}

func TestPublicSessionRevocationProofSurvivesCredentialCleanup(t *testing.T) {
	admin, database := fenceDatabase(t, ""), authorityDatabase(t, "identity")
	now := time.Now().UTC().Truncate(time.Microsecond)
	actor, _ := sessionRevocationActor(t, admin, now)
	input := publicRevocationInput(t, admin, actor, identity.RevokeSessionRequest, actor.SessionID)
	store := pgidentity.NewSessionRevocationStore(database)
	request, err := store.PrepareSessionRevocationRequest(t.Context(), actor, input, func() time.Time { return now })
	if err != nil || request.Validate() != nil {
		t.Fatalf("prepare durable request: %+v %v", request, err)
	}
	assertSessionLocallyRevoked(t, admin, actor.SessionID)
	if err := admin.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`DELETE FROM identity.session_refresh_token WHERE session_id=?`, actor.SessionID).Error; err != nil {
			return err
		}
		return tx.Exec(`DELETE FROM identity.user_session WHERE id=?`, actor.SessionID).Error
	}); err != nil {
		t.Fatal(err)
	}
	restarted := pgidentity.NewSessionRevocationStore(database)
	loaded, err := restarted.LoadSessionRevocationRequest(t.Context(), input)
	if err != nil || loaded.ID != request.ID || len(loaded.Revocations) != 1 || loaded.Revocations[0].Control != request.Revocations[0].Control {
		t.Fatalf("original proof did not recover after credential GC: %+v %v", loaded, err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*identity.SessionRevocationInput)
		want   error
	}{
		{"key", func(in *identity.SessionRevocationInput) { in.Key = uuid.New() }, writefence.ErrNotFound},
		{"proof", func(in *identity.SessionRevocationInput) { in.Proofs[0].TokenDigest = make([]byte, 32) }, writefence.ErrNotFound},
		{"csrf", func(in *identity.SessionRevocationInput) { in.Proofs[0].CSRFHash = make([]byte, 32) }, identity.ErrSessionCSRFInvalid},
		{"retarget", func(in *identity.SessionRevocationInput) { in.TargetID = uuid.New() }, identity.ErrForbidden},
		{"action", func(in *identity.SessionRevocationInput) {
			in.Action, in.TargetID = identity.RevokeOtherSessionsRequest, uuid.Nil
		}, identity.ErrForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := input
			changed.Proofs = append([]identity.SessionRevocationProof(nil), input.Proofs...)
			tc.mutate(&changed)
			if _, err := restarted.LoadSessionRevocationRequest(t.Context(), changed); !errors.Is(err, tc.want) {
				t.Fatalf("recovery authorization = %v, want %v", err, tc.want)
			}
		})
	}
	for _, role := range []string{"creation", "media", "execution"} {
		foreign := pgidentity.NewSessionRevocationStore(authorityDatabase(t, role))
		if _, err := foreign.LoadSessionRevocationRequest(t.Context(), input); err == nil {
			t.Fatalf("%s read Identity proof", role)
		}
		if _, err := foreign.PrepareSessionRevocationRequest(t.Context(), actor, input, func() time.Time { return now }); err == nil {
			t.Fatalf("%s prepared Identity request", role)
		}
		if err := foreign.CompleteSessionRevocationRequest(t.Context(), request, now); err == nil {
			t.Fatalf("%s completed Identity request", role)
		}
	}
	coordinator := writefence.NewCoordinator(pgfence.NewAuthorityStore(database), publicRevocationWriters(t))
	source := request.Revocations[0]
	expected := source.Control.Authority
	expected.Epoch--
	if _, err := coordinator.Set(t.Context(), expected, source.Control.CoordinationID, true); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteSessionRevocation(t.Context(), source, now); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := store.CompleteSessionRevocationRequest(t.Context(), request, now); err != nil {
			t.Fatal(err)
		}
	}
	var audits int
	if err := admin.Raw(`SELECT count(*) FROM identity.security_event WHERE request_id IN (?,?)`, request.ID.String(), source.Control.CoordinationID.String()).Scan(&audits).Error; err != nil || audits != 1 {
		t.Fatalf("single request duplicated source success audit: %d %v", audits, err)
	}
}

func TestPublicSessionRevocationBatchFreezesTargetsAndCompletion(t *testing.T) {
	admin, database := fenceDatabase(t, ""), authorityDatabase(t, "identity")
	now := time.Now().UTC().Truncate(time.Microsecond)
	actor, target := sessionRevocationActor(t, admin, now)
	store := pgidentity.NewSessionRevocationStore(database)
	// A previously accepted source remains a batch responsibility after session GC.
	old, err := store.PrepareSessionRevocation(t.Context(), actor, target, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if err := admin.Exec(`DELETE FROM identity.session_refresh_token WHERE session_id=?`, target).Error; err != nil {
		t.Fatal(err)
	}
	if err := admin.Exec(`DELETE FROM identity.user_session WHERE id=?`, target).Error; err != nil {
		t.Fatal(err)
	}
	if recovered, err := store.PrepareSessionRevocation(t.Context(), actor, target, func() time.Time { return now }); err != nil || recovered.Control != old.Control {
		t.Fatalf("live actor cannot recover original GC target: %+v %v", recovered, err)
	}
	input := publicRevocationInput(t, admin, actor, identity.RevokeOtherSessionsRequest, uuid.Nil)
	request, err := store.PrepareSessionRevocationRequest(t.Context(), actor, input, func() time.Time { return now })
	if err != nil || len(request.Revocations) != 1 || request.Revocations[0].Control != old.Control {
		t.Fatalf("batch lost existing source responsibility: %+v %v", request, err)
	}
	// Add a later session to the same account. A replay cannot select it.
	newID := uuid.New()
	newToken := sha256.Sum256(newID[:])
	if err := admin.Exec(`INSERT INTO identity.user_session(id,account_id,token_hash,credential_revision,created_at,last_active_at,absolute_expires_at,persistent,protocol_version) SELECT ?,account_id,?,credential_revision,created_at,last_active_at,absolute_expires_at,persistent,protocol_version FROM identity.user_session WHERE id=?`, newID, newToken[:], actor.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	replayed, err := store.PrepareSessionRevocationRequest(t.Context(), actor, input, func() time.Time { return now })
	if err != nil || replayed.ID != request.ID || len(replayed.Revocations) != 1 {
		t.Fatalf("replay reselected batch: %+v %v", replayed, err)
	}
	changed := input
	changed.Action, changed.TargetID = identity.RevokeSessionRequest, newID
	if _, err := store.PrepareSessionRevocationRequest(t.Context(), actor, changed, func() time.Time { return now }); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("accepted key was retargeted: %v", err)
	}
	if err := store.CompleteSessionRevocationRequest(t.Context(), request, now); err == nil {
		t.Fatal("request completed without source receipts")
	}
	truncated := request
	truncated.Revocations = nil
	if err := store.CompleteSessionRevocationRequest(t.Context(), truncated, now); err == nil {
		t.Fatal("incomplete target set completed request")
	}
	retargeted := request
	retargeted.Revocations = append([]identity.SessionRevocation(nil), request.Revocations...)
	retargeted.Revocations[0].Control.CoordinationID = uuid.New()
	if err := store.CompleteSessionRevocationRequest(t.Context(), retargeted, now); !errors.Is(err, writefence.ErrConflict) {
		t.Fatalf("substituted source identity completed request: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := store.CompleteSessionRevocationRequest(ctx, request, now); err == nil {
		t.Fatal("canceled completion succeeded")
	}
	coordinator := writefence.NewCoordinator(pgfence.NewAuthorityStore(database), publicRevocationWriters(t))
	for _, source := range request.Revocations {
		expected := source.Control.Authority
		expected.Epoch--
		if _, err := coordinator.Set(t.Context(), expected, source.Control.CoordinationID, true); err != nil {
			t.Fatal(err)
		}
		if err := store.CompleteSessionRevocation(t.Context(), source, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.CompleteSessionRevocationRequest(ctx, request, now); err == nil {
		t.Fatal("canceled completion committed after all source receipts")
	}
	pending, err := store.LoadSessionRevocationRequest(t.Context(), input)
	if err != nil || pending.CompletedAt != nil {
		t.Fatalf("canceled completion erased pending request: %+v %v", pending, err)
	}
	var pendingAudits int
	if err := admin.Raw(`SELECT count(*) FROM identity.security_event WHERE request_id=?`, request.ID.String()).Scan(&pendingAudits).Error; err != nil || pendingAudits != 0 {
		t.Fatalf("pending batch recorded success: %d %v", pendingAudits, err)
	}
	for range 2 {
		if err := store.CompleteSessionRevocationRequest(t.Context(), request, now); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := store.LoadSessionRevocationRequest(t.Context(), input)
	if err != nil || loaded.CompletedAt == nil || len(loaded.Revocations) != 1 {
		t.Fatalf("request completion missing: %+v %v", loaded, err)
	}
	var newRevoked bool
	if err := admin.Raw(`SELECT revoked_at IS NOT NULL FROM identity.user_session WHERE id=?`, newID).Scan(&newRevoked).Error; err != nil || newRevoked {
		t.Fatalf("later session entered fixed batch: %t %v", newRevoked, err)
	}
	var events int
	if err := admin.Raw(`SELECT count(*) FROM identity.security_event WHERE request_id=?`, request.ID.String()).Scan(&events).Error; err != nil || events != 1 {
		t.Fatalf("batch audit is not exactly once: %d %v", events, err)
	}
	if err := admin.Raw(`SELECT count(*) FROM identity.security_event WHERE request_id=?`, old.Control.CoordinationID.String()).Scan(&events).Error; err != nil || events != 1 {
		t.Fatalf("source audit duplicated: %d %v", events, err)
	}
}

func TestPublicSessionRevocationBatchClosesEveryOriginalTargetWithoutAddingProofs(t *testing.T) {
	admin, database := fenceDatabase(t, ""), authorityDatabase(t, "identity")
	now := time.Now().UTC().Truncate(time.Microsecond)
	actor, target := sessionRevocationActor(t, admin, now)
	other := uuid.New()
	digest := sha256.Sum256(other[:])
	// A locally revoked credential without a distributed source receipt remains
	// part of the original all-other-sessions policy, rather than disappearing.
	if err := admin.Exec(`INSERT INTO identity.user_session(id,account_id,token_hash,credential_revision,created_at,last_active_at,absolute_expires_at,persistent,protocol_version,revoked_at) SELECT ?,account_id,?,credential_revision,created_at,last_active_at,absolute_expires_at,persistent,protocol_version,? FROM identity.user_session WHERE id=?`, other, digest[:], now, actor.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	input := publicRevocationInput(t, admin, actor, identity.RevokeOtherSessionsRequest, uuid.Nil)
	refresh := sha256.Sum256(append(actor.SessionID[:], 'r'))
	input.Proofs = append(input.Proofs, identity.SessionRevocationProof{Kind: identity.SessionCredentialRefresh, TokenDigest: refresh[:], CSRFHash: input.Proofs[0].CSRFHash})
	store := pgidentity.NewSessionRevocationStore(database)
	request, err := store.PrepareSessionRevocationRequest(t.Context(), actor, input, func() time.Time { return now })
	if err != nil || len(request.Revocations) != 2 {
		t.Fatalf("batch did not close its full original set: %+v %v", request, err)
	}
	assertSessionLocallyRevoked(t, admin, target)
	assertSessionLocallyRevoked(t, admin, other)
	// A newly authenticated access token can observe the same live actor/key,
	// but must never enroll another proof in the already accepted request.
	changed := input
	changed.Proofs = append([]identity.SessionRevocationProof(nil), input.Proofs[:1]...)
	newAccess := sha256.Sum256([]byte(uuid.NewString()))
	changed.Proofs[0].TokenDigest = newAccess[:]
	if replay, err := store.PrepareSessionRevocationRequest(t.Context(), actor, changed, func() time.Time { return now }); err != nil || replay.ID != request.ID {
		t.Fatalf("authenticated replay changed request: %+v %v", replay, err)
	}
	if _, err := store.LoadSessionRevocationRequest(t.Context(), changed); !errors.Is(err, writefence.ErrNotFound) {
		t.Fatalf("replay enrolled a new recovery proof: %v", err)
	}
	if err := admin.Exec(`DELETE FROM identity.session_refresh_token WHERE session_id=?`, actor.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	refreshOnly := input
	refreshOnly.Primary, refreshOnly.Proofs = "", input.Proofs[1:]
	loaded, err := store.LoadSessionRevocationRequest(t.Context(), refreshOnly)
	if err != nil || loaded.ID != request.ID || len(loaded.Revocations) != 2 {
		t.Fatalf("verified secondary refresh did not survive credential cleanup: %+v %v", loaded, err)
	}
	mixed := input
	mixed.Proofs = append([]identity.SessionRevocationProof(nil), input.Proofs...)
	mixed.Proofs[0].CSRFHash = make([]byte, 32)
	if loaded, err := store.LoadSessionRevocationRequest(t.Context(), mixed); err != nil || loaded.ID != request.ID {
		t.Fatalf("one complete original proof failed to recover full batch: %+v %v", loaded, err)
	}
	var proofs int
	if err := admin.Raw(`SELECT count(*) FROM identity.session_revocation_request_proof WHERE request_id=?`, request.ID).Scan(&proofs).Error; err != nil || proofs != 2 {
		t.Fatalf("Load/Prepare changed accepted proof set: %d %v", proofs, err)
	}
}

func publicRevocationWriters(t *testing.T) authorityLocalWriters {
	t.Helper()
	writers := make(authorityLocalWriters)
	for _, writer := range writefence.Writers() {
		writers[writer] = pgfence.New(authorityDatabase(t, string(writer)))
	}
	return writers
}

func TestPublicSessionRevocationBatchRollsBackAllSources(t *testing.T) {
	admin, database := fenceDatabase(t, ""), authorityDatabase(t, "identity")
	now := time.Now().UTC().Truncate(time.Microsecond)
	actor, target := sessionRevocationActor(t, admin, now)
	input := publicRevocationInput(t, admin, actor, identity.RevokeOtherSessionsRequest, uuid.Nil)
	otherTarget := uuid.New()
	otherToken := sha256.Sum256(otherTarget[:])
	if err := admin.Exec(`INSERT INTO identity.user_session(id,account_id,token_hash,credential_revision,created_at,last_active_at,absolute_expires_at,persistent,protocol_version) SELECT ?,account_id,?,credential_revision,created_at,last_active_at,absolute_expires_at,persistent,protocol_version FROM identity.user_session WHERE id=?`, otherTarget, otherToken[:], actor.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	first, conflicting := target, otherTarget
	if first.String() > conflicting.String() {
		first, conflicting = conflicting, first
	}
	// Force the second target to conflict after the first target's complete local
	// closure and source journal have already run inside this same transaction.
	if _, err := pgfence.NewAuthorityStore(database).Begin(t.Context(), writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Session, ID: conflicting}, uuid.New(), false); err != nil {
		t.Fatal(err)
	}
	store := pgidentity.NewSessionRevocationStore(database)
	if _, err := store.PrepareSessionRevocationRequest(t.Context(), actor, input, func() time.Time { return now }); err == nil {
		t.Fatal("conflicting target committed batch")
	}
	for _, table := range []string{"session_revocation", "session_revocation_request"} {
		var count int
		if err := admin.Raw(`SELECT count(*) FROM identity.`+table+` WHERE account_id=?`, actor.ID).Scan(&count).Error; err != nil || count != 0 {
			t.Fatalf("partial %s committed: %d %v", table, count, err)
		}
	}
	var active int
	if err := admin.Raw(`SELECT count(*) FROM identity.user_session WHERE account_id=? AND revoked_at IS NULL`, actor.ID).Scan(&active).Error; err != nil || active != 3 {
		t.Fatalf("batch partially closed sessions: %d %v", active, err)
	}
	if err := admin.Raw(`SELECT count(*) FROM identity.session_refresh_token WHERE session_id=? AND revoked_at IS NULL`, target).Scan(&active).Error; err != nil || active != 1 {
		t.Fatalf("batch partially closed refresh chain: %d %v", active, err)
	}
	if err := admin.Raw(`SELECT count(*) FROM infra.authority_head WHERE service_owner='identity' AND authority_kind='session' AND authority_id=?`, first).Scan(&active).Error; err != nil || active != 0 {
		t.Fatalf("earlier target authority escaped rollback: %d %v", active, err)
	}
	if err := database.Transaction(func(tx *gorm.DB) error {
		_, err := pgidentity.NewSessionRevocationStore(tx).PrepareSessionRevocationRequest(t.Context(), actor, input, func() time.Time { return now })
		if !errors.Is(err, writefence.ErrTransaction) {
			t.Fatalf("request accepted outer transaction: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPublicSessionRevocationOnlyPersistsAuthenticatedProofs(t *testing.T) {
	for _, mode := range []string{"access-valid-refresh", "access-foreign-refresh", "access-revoked-refresh", "refresh-revoked", "refresh-untrusted-access", "legacy", "legacy-bad-token", "legacy-wrong-protocol", "refresh-foreign", "refresh-batch", "refresh-other-target", "wrong-csrf", "stale-actor"} {
		t.Run(mode, func(t *testing.T) {
			admin, database := fenceDatabase(t, ""), authorityDatabase(t, "identity")
			now := time.Now().UTC().Truncate(time.Microsecond)
			actor, target := sessionRevocationActor(t, admin, now)
			input := publicRevocationInput(t, admin, actor, identity.RevokeSessionRequest, actor.SessionID)
			refresh := sha256.Sum256(append(actor.SessionID[:], 'r'))
			otherRefresh := sha256.Sum256(append(target[:], 'r'))
			refreshProof := identity.SessionRevocationProof{Kind: identity.SessionCredentialRefresh, TokenDigest: refresh[:], CSRFHash: input.Proofs[0].CSRFHash}
			wantCount, wantErr := 1, false
			switch mode {
			case "access-valid-refresh":
				input.Proofs = append(input.Proofs, refreshProof)
				wantCount = 2
			case "access-foreign-refresh":
				refreshProof.TokenDigest = otherRefresh[:]
				input.Proofs = append(input.Proofs, refreshProof)
			case "access-revoked-refresh", "refresh-revoked":
				if err := admin.Exec(`UPDATE identity.session_refresh_token SET revoked_at=? WHERE session_id=?`, now, actor.SessionID).Error; err != nil {
					t.Fatal(err)
				}
				if mode == "refresh-revoked" {
					input.Primary, input.Proofs = identity.SessionCredentialRefresh, []identity.SessionRevocationProof{refreshProof}
					wantErr = true
				} else {
					input.Proofs = append(input.Proofs, refreshProof)
				}
			case "refresh-untrusted-access":
				input.Primary = identity.SessionCredentialRefresh
				input.Proofs = append(input.Proofs, refreshProof)
			case "legacy", "legacy-bad-token", "legacy-wrong-protocol":
				input.Primary, input.Proofs[0].Kind = identity.SessionCredentialLegacy, identity.SessionCredentialLegacy
				digest := sha256.Sum256(actor.SessionID[:])
				input.Proofs[0].TokenDigest = digest[:]
				if mode != "legacy-wrong-protocol" {
					if err := admin.Exec(`UPDATE identity.user_session SET protocol_version=1 WHERE id=?`, actor.SessionID).Error; err != nil {
						t.Fatal(err)
					}
				}
				if mode == "legacy-bad-token" {
					input.Proofs[0].TokenDigest = make([]byte, 32)
				}
				wantErr = mode != "legacy"
			case "refresh-foreign", "refresh-batch", "refresh-other-target":
				input.Primary, input.Proofs = identity.SessionCredentialRefresh, []identity.SessionRevocationProof{refreshProof}
				if mode == "refresh-foreign" {
					input.Proofs[0].TokenDigest = otherRefresh[:]
				}
				if mode == "refresh-batch" {
					input.Action, input.TargetID = identity.RevokeOtherSessionsRequest, uuid.Nil
				}
				if mode == "refresh-other-target" {
					input.TargetID = target
				}
				wantErr = true
			case "wrong-csrf":
				input.Proofs[0].CSRFHash = make([]byte, 32)
				wantErr = true
			case "stale-actor":
				actor.CredentialEpoch++
				wantErr = true
			}
			store := pgidentity.NewSessionRevocationStore(database)
			request, err := store.PrepareSessionRevocationRequest(t.Context(), actor, input, func() time.Time { return now })
			if wantErr {
				if err == nil {
					t.Fatal("invalid original proof authorized request")
				}
				var count int
				if err := admin.Raw(`SELECT count(*) FROM identity.session_revocation_request WHERE account_id=?`, actor.ID).Scan(&count).Error; err != nil || count != 0 {
					t.Fatalf("rejected proof persisted request: %d %v", count, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var count int
			if err := admin.Raw(`SELECT count(*) FROM identity.session_revocation_request_proof WHERE request_id=?`, request.ID).Scan(&count).Error; err != nil || count != wantCount {
				t.Fatalf("stored unverified proof: %d, want %d: %v", count, wantCount, err)
			}
			if mode == "refresh-untrusted-access" || mode == "access-foreign-refresh" {
				untrusted := input
				untrusted.Primary = ""
				if mode == "refresh-untrusted-access" {
					untrusted.Proofs = input.Proofs[:1]
				} else {
					untrusted.Proofs = input.Proofs[1:]
				}
				if _, err := store.LoadSessionRevocationRequest(t.Context(), untrusted); !errors.Is(err, writefence.ErrNotFound) {
					t.Fatalf("unverified secondary recovered request: %v", err)
				}
			}
		})
	}
}

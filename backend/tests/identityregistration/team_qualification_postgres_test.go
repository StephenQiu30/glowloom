package identityregistration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"

	creationsource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/identitysource"
	pgcreation "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/pgsource"
	creation "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	identityrpc "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/grpcauthority"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

func acceptedTeamQualificationFixture(t *testing.T) (*registrationFixture, *registrationFixture, identity.TeamQualificationRequest) {
	t.Helper()
	f, password := passwordFixture(t)
	candidate, candidatePassword := passwordFixture(t)
	target, err := f.auth.Current(t.Context(), password.Credential.LegacyToken)
	if err != nil {
		t.Fatal(err)
	}
	other, err := candidate.auth.Current(t.Context(), candidatePassword.Credential.LegacyToken)
	if err != nil {
		t.Fatal(err)
	}
	state, err := f.writers.Set(t.Context(), writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Account, ID: target.User.ID, Epoch: 1}, uuid.New(), true)
	if err != nil {
		t.Fatal(err)
	}
	request := identity.TeamQualificationRequest{RequestID: uuid.New(), TeamID: uuid.New(), CandidateID: other.User.ID, SnapshotSHA256: strings.Repeat("b", 64)}
	target.User.PasswordHash = ""
	body, err := json.Marshal(map[string]any{"Actor": target.User, "User": target.User, "Reason": "synthetic accepted deletion qualification"})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.idb.Exec(`INSERT INTO identity.account_qualification_change(request_id,actor_id,actor_session_id,target_id,action,input_digest,proof_digest,candidate,close_coordination_id,close_expected_epoch,accepted_at) VALUES(?,?,?,?,'delete',?,?,?::jsonb,?,1,statement_timestamp())`, request.RequestID, target.User.ID, target.Session.ID, target.User.ID, bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32), string(body), state.Control.CoordinationID).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupSyntheticQualification(t, request.RequestID) })
	return f, candidate, request
}

func cleanupSyntheticQualification(t *testing.T, id uuid.UUID) {
	t.Helper()
	name := os.Getenv("GLOWLOOM_TEST_REGISTRATION_DB_NAME")
	if !regexp.MustCompile(`^glowloom_[a-z0-9_]+_test$`).MatchString(name) {
		t.Error("refused synthetic cleanup outside isolated test database")
		return
	}
	connection, err := db.Open(context.Background(), "postgres://postgres@127.0.0.1:52080/"+name+"?sslmode=disable", noop.NewTracerProvider())
	if err != nil {
		t.Error("open isolated synthetic cleanup capability failed")
		return
	}
	defer func() { _ = connection.Close() }()
	if err := connection.DB.Transaction(func(tx *gorm.DB) error {
		var owned bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM identity.account_qualification_change WHERE request_id=? AND candidate->>'Reason'='synthetic accepted deletion qualification' AND input_digest=? AND proof_digest=?)`, id, bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32)).Scan(&owned).Error; err != nil {
			return err
		}
		if !owned {
			return errors.New("synthetic qualification cleanup ownership mismatch")
		}
		// This dedicated fixture-only cleanup deletes no real account, session,
		// business effect or policy head and does not change native role grants.
		if err := tx.Exec(`UPDATE identity.team_administrator_qualification SET previous_proof_id=NULL WHERE request_id=?`, id).Error; err != nil {
			return err
		}
		if err := tx.Exec(`DELETE FROM identity.team_administrator_qualification WHERE request_id=?`, id).Error; err != nil {
			return err
		}
		if err := tx.Exec(`DELETE FROM infra.task_work WHERE service_owner='creation' AND kind='creation.account_governance' AND business_id=?`, id.String()).Error; err != nil {
			return err
		}
		if err := tx.Exec(`DELETE FROM workspace.account_governance_preparation WHERE request_id=?`, id).Error; err != nil {
			return err
		}
		return tx.Exec(`DELETE FROM identity.account_qualification_change WHERE request_id=?`, id).Error
	}); err != nil {
		t.Errorf("isolated synthetic qualification cleanup: %v", err)
	}
}

func TestTeamQualificationActualSourceSafetyCloseAndSuccessor(t *testing.T) {
	f, candidate, request := acceptedTeamQualificationFixture(t)
	store := pgidentity.NewTeamQualificationStore(f.idb)
	service := identity.NewTeamQualifications(store)
	positive, err := service.Issue(t.Context(), request)
	if err != nil || !positive.Eligible {
		t.Fatalf("eligible source purpose: %v", err)
	}
	if count(t, f.idb, `SELECT count(*) FROM infra.authority_admission WHERE service_owner=infra.runtime_owner() AND id=? AND request_sha256=? AND target_writer='creation'`, positive.ID, positive.Command.SHA256) != 1 {
		t.Fatal("positive purpose lacks its exact target-Creation source admission")
	}
	writer := pgfence.New(f.cdb)
	if _, err := writer.Admit(t.Context(), positive.Command); err != nil {
		t.Fatal(err)
	}
	// An accepted proof must not pin eligibility or block safety revocation.
	if _, err := candidate.writers.Set(t.Context(), positive.Control.Authority, uuid.New(), true); err != nil {
		t.Fatal(err)
	}
	if err := f.cdb.Transaction(func(tx *gorm.DB) error { return writer.Guard(t.Context(), tx, positive.Command) }); !errors.Is(err, writefence.ErrFenced) {
		t.Fatalf("old candidate proof survived safety close: %v", err)
	}
	request.PreviousProofID = positive.ID
	if _, err := service.Issue(t.Context(), request); !errors.Is(err, writefence.ErrUnavailable) {
		t.Fatalf("active closed candidate became a negative vote: %v", err)
	}
	if err := candidate.idb.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, request.CandidateID).Error; err != nil {
		t.Fatal(err)
	}
	negative, err := service.Issue(t.Context(), request)
	if err != nil || negative.Eligible || negative.PreviousProofID != positive.ID || negative.Control.Epoch != 2 {
		t.Fatalf("stable inactive successor: %v", err)
	}
	if count(t, f.idb, `SELECT count(*) FROM infra.authority_admission WHERE service_owner=infra.runtime_owner() AND id=?`, negative.ID) != 0 {
		t.Fatal("negative created a normal admission")
	}
	old, err := service.Resolve(t.Context(), positive.ID)
	if err != nil || old.Command.SHA256 != positive.Command.SHA256 || old.Control != positive.Control || !old.Eligible {
		t.Fatal("successor rewrote original positive proof")
	}
	if _, err := identity.NewTeamQualifications(pgidentity.NewTeamQualificationStore(f.cdb)).Resolve(t.Context(), positive.ID); !errors.Is(err, writefence.ErrUnavailable) {
		t.Fatal("foreign database role read Identity qualification")
	}
}

func TestTeamQualificationActualMTLSAndGeneratedOnlyCreatorClient(t *testing.T) {
	f, candidate, r := acceptedTeamQualificationFixture(t)
	tokens := map[writefence.Writer]string{}
	names := map[writefence.Writer]string{}
	for _, writer := range writefence.Writers() {
		tokens[writer] = strings.Repeat("team-qualification-", 3) + string(writer)
		names[writer] = string(writer) + ".qualification"
	}
	service := identity.NewTeamQualifications(pgidentity.NewTeamQualificationStore(f.idb))
	server, err := identityrpc.NewMTLSServer(f.auth, pgidentity.NewSessionAdmissionStore(f.idb), tokens, names, time.Now, f.tls.server(t, "identity.qualification.server"), service)
	if err != nil {
		t.Fatal(err)
	}
	address := serve(t, server)
	client, err := creationsource.New(address, tokens[writefence.WriterCreation], f.tls.client(t, "identity.qualification.server", names[writefence.WriterCreation]))
	if err != nil {
		t.Fatal(err)
	}
	intent, err := client.ReadAccountGovernanceIntent(t.Context(), r.RequestID)
	if err != nil || intent.CloseControl.Epoch != 2 || !intent.CloseControl.Blocked {
		t.Fatalf("actual source intent: %v", err)
	}
	request := creation.GovernanceQualificationRequest{RequestID: r.RequestID, TeamID: r.TeamID, CandidateID: r.CandidateID, SnapshotSHA256: r.SnapshotSHA256}
	proof, err := client.ReadGovernanceQualification(t.Context(), request)
	if err != nil || !proof.Eligible || proof.Control.Epoch != 1 {
		t.Fatalf("actual source qualification: %v", err)
	}
	if _, err := candidate.writers.Set(t.Context(), proof.Control.Authority, uuid.New(), true); err != nil {
		t.Fatal(err)
	}
	request.PreviousProofID = proof.ID
	if _, err := client.ReadGovernanceQualification(t.Context(), request); !errors.Is(err, writefence.ErrUnavailable) {
		t.Fatalf("active blocked successor became negative: %v", err)
	}
	if err := candidate.idb.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, r.CandidateID).Error; err != nil {
		t.Fatal(err)
	}
	negative, err := client.ReadGovernanceQualification(t.Context(), request)
	if err != nil || negative.Eligible || negative.PreviousProofID != proof.ID {
		t.Fatalf("actual explicit successor: %v", err)
	}
	wrong, err := creationsource.New(address, tokens[writefence.WriterMedia], f.tls.client(t, "identity.qualification.server", names[writefence.WriterMedia]))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wrong.ReadAccountGovernanceIntent(t.Context(), r.RequestID); err == nil {
		t.Fatal("Media identity used Creation-only governance qualification")
	}
}

type directGovernanceQualification struct{ source *identity.TeamQualifications }

func creationIntent(p identity.AccountQualificationIntent) creation.AccountGovernanceIntent {
	return creation.AccountGovernanceIntent{RequestID: p.RequestID, AccountID: p.AccountID, Action: p.Action, InputSHA256: p.InputSHA256, CloseControl: p.CloseControl, EligibleAfter: p.EligibleAfter}
}
func (p directGovernanceQualification) ReadAccountGovernanceIntent(ctx context.Context, id uuid.UUID) (creation.AccountGovernanceIntent, error) {
	v, err := p.source.Intent(ctx, id)
	return creationIntent(v), err
}
func (p directGovernanceQualification) ReadGovernanceQualification(ctx context.Context, r creation.GovernanceQualificationRequest) (creation.GovernanceQualification, error) {
	v, err := p.source.Issue(ctx, identity.TeamQualificationRequest{RequestID: r.RequestID, TeamID: r.TeamID, CandidateID: r.CandidateID, PreviousProofID: r.PreviousProofID, SnapshotSHA256: r.SnapshotSHA256})
	if err != nil {
		return creation.GovernanceQualification{}, err
	}
	return creation.GovernanceQualification{ID: v.ID, PreviousProofID: v.PreviousProofID, Intent: creationIntent(v.Intent), TeamID: v.TeamID, CandidateID: v.CandidateID, SnapshotSHA256: v.SnapshotSHA256, Eligible: v.Eligible, Control: v.Control, Command: v.Command}, nil
}

func TestTeamDutiesRejectionPreservesOtherMemberEpochOneCommand(t *testing.T) {
	f, other, request := acceptedTeamQualificationFixture(t)
	qualifier := identity.NewTeamQualifications(pgidentity.NewTeamQualificationStore(f.idb))
	intent, err := qualifier.Intent(t.Context(), request.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	team, scope, organization := uuid.New(), uuid.New(), uuid.New()
	if err := f.cdb.Exec(`INSERT INTO workspace.organization(id,name) VALUES(?,'governance isolation fixture')`, organization).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.cdb.Exec(`INSERT INTO workspace.team(id,name,created_by) VALUES(?,'governance isolation fixture',?)`, team, intent.AccountID).Error; err != nil {
		t.Fatal(err)
	}
	for _, member := range []struct {
		id   uuid.UUID
		role string
	}{{intent.AccountID, "team_admin"}, {request.CandidateID, "member"}} {
		if err := f.cdb.Exec(`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,?,'active',statement_timestamp(),'created')`, team, member.id, member.role).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := f.cdb.Exec(`INSERT INTO workspace.creation_scope(id,kind,team_id,physical_scope_id) VALUES(?,'team',?,?)`, scope, team, organization).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.creationWriters.Set(t.Context(), writefence.Authority{Issuer: writefence.Creation, Kind: writefence.Scope, ID: scope, Epoch: 0}, uuid.New(), false); err != nil {
		t.Fatal(err)
	}
	var session struct{ ID uuid.UUID }
	if err := other.idb.Raw(`SELECT id FROM identity.user_session WHERE account_id=? AND revoked_at IS NULL`, request.CandidateID).Scan(&session).Error; err != nil {
		t.Fatal(err)
	}
	command := writefence.Command{ID: uuid.New(), SHA256: strings.Repeat("e", 64), Action: "creation.independent_canvas.create", Authorities: []writefence.Authority{{Issuer: writefence.Identity, Kind: writefence.Account, ID: request.CandidateID, Epoch: 1}, {Issuer: writefence.Identity, Kind: writefence.Session, ID: session.ID, Epoch: 1}, {Issuer: writefence.Creation, Kind: writefence.Scope, ID: scope, Epoch: 1}}}
	writer := pgfence.New(f.cdb)
	if _, err := writer.Admit(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	store := pgcreation.NewAccountGovernanceStore(f.cdb)
	service, err := creation.NewAccountGovernance(store, directGovernanceQualification{source: qualifier}, f.creationWriters)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := service.Prepare(t.Context(), creationIntent(intent))
	if err != nil || !receipt.RejectedDuties || len(receipt.BlockingTeams) != 1 || receipt.BlockingTeams[0] != team {
		t.Fatalf("actual last-team-admin duties rejection: %v", err)
	}
	if count(t, f.cdb, `SELECT count(*) FROM infra.authority_head WHERE service_owner=infra.runtime_owner() AND issuer='creation' AND authority_kind='scope' AND authority_id=? AND epoch=1 AND NOT blocked AND complete`, scope) != 1 {
		t.Fatal("duties rejection advanced or closed another member's shared scope")
	}
	if err := f.cdb.Transaction(func(tx *gorm.DB) error {
		if err := writer.Guard(t.Context(), tx, command); err != nil {
			return err
		}
		return writer.Complete(t.Context(), tx, command, json.RawMessage(`{"other_member":"committed_original_epoch_one"}`))
	}); err != nil {
		t.Fatalf("other member's original admitted command was permanently discarded by duties rejection: %v", err)
	}
	if count(t, f.cdb, `SELECT count(*) FROM workspace.team WHERE id=? AND revision=1 AND auth_epoch=1 AND NOT governance_blocked`, team) != 1 {
		t.Fatal("duties rejection mutated team facts")
	}
	if _, err := service.Prepare(t.Context(), creationIntent(intent)); err != nil {
		t.Fatal(err)
	}
}

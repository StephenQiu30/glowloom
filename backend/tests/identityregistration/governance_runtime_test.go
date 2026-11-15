package identityregistration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	creationrpc "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/grpcaccountgovernance"
	creationsource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/identitysource"
	pgcreation "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/pgsource"
	creation "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	identityrpc "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/grpcauthority"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/http"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/identity/app"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	coordinationpb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/coordination"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	fencerpc "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/grpctransport"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

// The fault is after the real restricted Creation transaction has committed.
// It drops every response while enabled, including recovery worker attempts.
type lostGovernanceReceipt struct {
	service *creation.AccountGovernance
	lose    atomic.Bool
}

func (p *lostGovernanceReceipt) Prepare(ctx context.Context, intent creation.AccountGovernanceIntent) (creation.AccountGovernanceReceipt, error) {
	r, err := p.service.Prepare(ctx, intent)
	if err == nil && p.lose.Load() {
		return creation.AccountGovernanceReceipt{}, writefence.ErrUnavailable
	}
	return r, err
}
func (p *lostGovernanceReceipt) Resolve(ctx context.Context, id uuid.UUID) (creation.AccountGovernanceReceipt, error) {
	r, err := p.service.Resolve(ctx, id)
	if err == nil && p.lose.Load() {
		return creation.AccountGovernanceReceipt{}, writefence.ErrUnavailable
	}
	return r, err
}

func TestIdentityRuntimeTeamGovernanceRecoversLostReceiptsAndRestorationACK(t *testing.T) {
	f, recovery, credential, actorID := governanceSourceFixture(t)
	target, targetPassword := passwordFixture(t)
	current, err := target.auth.Current(t.Context(), targetPassword.Credential.LegacyToken)
	if err != nil {
		t.Fatal(err)
	}
	team, scope, organization := uuid.New(), uuid.New(), uuid.New()
	if err := f.cdb.Exec(`INSERT INTO workspace.organization(id,name) VALUES(?,'native governance fixture')`, organization).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.cdb.Exec(`INSERT INTO workspace.team(id,name,created_by) VALUES(?,'native governance fixture',?)`, team, current.User.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.cdb.Exec(`INSERT INTO workspace.creation_scope(id,kind,team_id,physical_scope_id) VALUES(?,'team',?,?)`, scope, team, organization).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.cdb.Exec(`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'team_admin','active',statement_timestamp(),'created')`, team, current.User.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.creationWriters.Set(t.Context(), writefence.Authority{Issuer: writefence.Creation, Kind: writefence.Scope, ID: scope, Epoch: 0}, uuid.New(), false); err != nil {
		t.Fatal(err)
	}
	cfg, _ := runtimeConfig(t, f)
	// Creation calls the actual Identity qualification RPC using its own audience.
	qualifier, err := identityrpc.NewMTLSServer(f.auth, pgidentity.NewSessionAdmissionStore(f.idb), cfg.AuthorityTokens, cfg.AuthorityPeerNames, time.Now, f.tls.server(t, "identity.governance.qualification"), identity.NewTeamQualifications(pgidentity.NewTeamQualificationStore(f.idb)))
	if err != nil {
		t.Fatal(err)
	}
	source, err := creationsource.New(serve(t, qualifier), cfg.AuthorityTokens[writefence.WriterCreation], f.tls.client(t, "identity.governance.qualification", cfg.AuthorityPeerNames[writefence.WriterCreation]))
	if err != nil {
		t.Fatal(err)
	}
	business, err := creation.NewAccountGovernance(pgcreation.NewAccountGovernanceStore(f.cdb), source, f.creationWriters)
	if err != nil {
		t.Fatal(err)
	}
	loss := &lostGovernanceReceipt{service: business}
	peer := cfg.CreationGovernance
	server, err := creationrpc.NewServer(loss, peer.Token, cfg.TLS.OwnName, f.tls.server(t, peer.ServerName))
	if err != nil {
		t.Fatal(err)
	}
	peer.Address = serve(t, server)
	cfg.CreationGovernance = peer
	// Reject configurations that reuse a registration or writer audience.
	invalid := cfg
	invalid.CreationGovernance.Token = cfg.CreationRegistration.Token
	if invalid.Validate() == nil {
		t.Fatal("governance reused registration audience")
	}
	invalid = cfg
	invalid.CreationGovernance = identityapp.Peer{}
	if invalid.Validate() == nil {
		t.Fatal("missing governance peer accepted")
	}
	var loseRestore atomic.Bool
	media := cfg.Writers[writefence.WriterMedia]
	handler, err := fencerpc.NewServer(writefence.WriterMedia, pgfence.New(registrationDB(t, "media")), media.Token, strings.Repeat("creation-writer-", 3)+"media")
	if err != nil {
		t.Fatal(err)
	}
	writer := grpc.NewServer(grpc.Creds(f.tls.server(t, media.ServerName)), grpc.UnaryInterceptor(func(ctx context.Context, request any, _ *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		response, err := next(ctx, request)
		r, ok := request.(*coordinationpb.InstallRequest)
		if err == nil && ok && r.Control != nil && r.Control.Authority != nil && r.Control.Authority.Kind == coordinationpb.AuthorityKind_AUTHORITY_KIND_ACCOUNT && !r.Control.Blocked && loseRestore.Load() {
			return nil, status.Error(codes.Unavailable, "synthetic committed restoration ACK lost")
		}
		return response, err
	}))
	coordinationpb.RegisterWriterFenceServer(writer, handler)
	media.Address = serve(t, writer)
	cfg.Writers[writefence.WriterMedia] = media
	var address string
	var stop func()
	start := func() {
		app, err := identityapp.New(t.Context(), cfg, zap.NewNop())
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- app.Run(ctx) }()
		address, _, _ = app.Addresses()
		stop = func() {
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Error(err)
				}
			case <-time.After(10 * time.Second):
				t.Error("native governance runtime did not join workers/servers")
			}
		}
	}
	start()
	t.Cleanup(func() {
		if stop != nil {
			stop()
		}
	})
	client := &http.Client{Timeout: 15 * time.Second}
	t.Cleanup(client.CloseIdleConnections)
	cookies := []*http.Cookie{{Name: "glowloom_session", Value: credential.Credential.LegacyToken}}
	csrf := credential.Credential.CSRFToken
	send := func(path string, key uuid.UUID, body any, want int) {
		t.Helper()
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+address+path, bytes.NewReader(encoded))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Origin", cfg.PublicOrigin)
		r.Header.Set("Idempotency-Key", key.String())
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", csrf)
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		response, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != want {
			body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
			t.Fatalf("%s status=%d want=%d: %s", path, response.StatusCode, want, body)
		}
		if len(response.Cookies()) != 0 {
			t.Fatal("governance issued credentials during mutation/recovery")
		}
	}
	path := func(action string) string { return "/api/platform/users/" + current.User.ID.String() + "/" + action }
	body := identityhttp.GovernanceMutation{ExpectedRevision: current.User.Revision, Reason: "native exact resource governance"}
	missingCSRFKey, staleRevisionKey, selfKey, forbiddenKey := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	csrf = ""
	send(path("disable"), missingCSRFKey, body, 403)
	csrf = credential.Credential.CSRFToken
	stale := body
	stale.ExpectedRevision++
	send(path("disable"), staleRevisionKey, stale, 409)
	cookies = []*http.Cookie{{Name: "glowloom_session", Value: targetPassword.Credential.LegacyToken}}
	csrf = targetPassword.Credential.CSRFToken
	send(path("disable"), selfKey, body, 422)
	send("/api/platform/users/"+actorID.String()+"/disable", forbiddenKey, body, 403)
	cookies = []*http.Cookie{{Name: "glowloom_session", Value: credential.Credential.LegacyToken}}
	csrf = credential.Credential.CSRFToken
	if count(t, f.idb, `SELECT count(*) FROM identity.account_qualification_change WHERE request_id IN ?`, []uuid.UUID{missingCSRFKey, staleRevisionKey, selfKey, forbiddenKey}) != 0 {
		t.Fatal("invalid new authorization enrolled a durable qualification change")
	}
	key := uuid.New()
	disableKey := key
	loss.lose.Store(true)
	send(path("disable"), key, body, 503)
	if count(t, f.cdb, `SELECT count(*) FROM workspace.account_governance_preparation WHERE request_id=? AND verdict='ready' AND completed_at IS NOT NULL`, key) != 1 {
		t.Fatal("response loss did not follow real Creation commit")
	}
	if count(t, f.idb, `SELECT count(*) FROM identity."user" WHERE id=? AND status='active' AND revision=?`, current.User.ID, current.User.Revision) != 1 {
		t.Fatal("Identity applied disable before exact Creation receipt")
	}
	if count(t, f.cdb, `SELECT count(*) FROM workspace.team WHERE id=? AND governance_blocked AND revision=2 AND auth_epoch=2`, team) != 1 {
		t.Fatal("real Creation freeze not committed")
	}
	// Accepted proof recovery must survive the original actor losing its session.
	if err := f.idb.Exec(`UPDATE identity.user_session SET revoked_at=statement_timestamp() WHERE account_id=?`, actorID).Error; err != nil {
		t.Fatal(err)
	}
	stop()
	stop = nil
	loss.lose.Store(false)
	start()
	send(path("disable"), key, body, 204)
	send(path("disable"), key, body, 204)
	if count(t, f.idb, `SELECT count(*) FROM identity."user" WHERE id=? AND status='disabled' AND revision=? AND session_epoch=? AND platform_epoch=?`, current.User.ID, current.User.Revision+1, current.User.SessionEpoch+1, current.User.PlatformEpoch+1) != 1 {
		t.Fatal("recovered disable counters differ from original contract")
	}
	// Use a fresh real HTTP session for subsequent new commands.
	var actorEmail string
	if err := f.idb.Raw(`SELECT email_key FROM identity."user" WHERE id=?`, actorID).Scan(&actorEmail).Error; err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(identityhttp.EmailLoginRequest{Email: actorEmail, Password: credential.CurrentPassword})
	r, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+address+"/api/sessions", bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Origin", cfg.PublicOrigin)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Idempotency-Key", uuid.NewString())
	response, err := client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 201 {
		t.Fatalf("new governance actor login=%d", response.StatusCode)
	}
	var session identityhttp.EmailSessionView
	if err := json.NewDecoder(response.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	cookies, csrf = response.Cookies(), session.CSRFToken
	send("/api/me/reauthentication", uuid.New(), identityhttp.ReauthenticationRequest{Password: credential.CurrentPassword}, 204)
	body.ExpectedRevision++
	enableKey := uuid.New()
	send(path("enable"), enableKey, body, 204)
	if count(t, f.cdb, `SELECT count(*) FROM workspace.team WHERE id=? AND governance_blocked AND revision=3 AND auth_epoch=3`, team) != 1 {
		t.Fatal("enable unexpectedly unfroze team")
	}
	body.ExpectedRevision++
	key = uuid.New()
	loseRestore.Store(true)
	send(path("delete"), key, body, 503)
	if count(t, f.idb, `SELECT count(*) FROM identity.account_qualification_change WHERE request_id=? AND outcome='pending' AND creation_receipt IS NOT NULL AND reopen_purpose='restore_rejection' AND completed_at IS NULL`, key) != 1 {
		t.Fatal("duties conflict escaped before restoration ACKs")
	}
	if count(t, f.idb, `SELECT count(*) FROM identity."user" WHERE id=? AND NOT is_delete AND revision=?`, current.User.ID, body.ExpectedRevision) != 1 {
		t.Fatal("pending duties rejection mutated account")
	}
	loseRestore.Store(false)
	send(path("delete"), key, body, 409)
	send(path("delete"), key, body, 409)
	if err := recovery.ResumeGovernance(t.Context(), key); err != nil {
		t.Fatalf("terminal duties rejection must complete its recovery task: %v", err)
	}
	if count(t, f.cdb, `SELECT count(*) FROM infra.authority_head WHERE service_owner='creation' AND issuer='creation' AND authority_kind='scope' AND authority_id=? AND epoch=2 AND blocked AND complete`, scope) != 1 {
		t.Fatal("duties rejection changed shared scope epoch")
	}
	// Remove the real team duty through Creation before a distinct delete command.
	if err := f.cdb.Exec(`UPDATE workspace.team_member SET role='member',revision=revision+1,auth_epoch=auth_epoch+1 WHERE team_id=? AND user_id=?`, team, current.User.ID).Error; err != nil {
		t.Fatal(err)
	}
	deleteKey := uuid.New()
	send(path("delete"), deleteKey, body, 204)
	send(path("delete"), deleteKey, body, 204)
	if count(t, f.idb, `SELECT count(*) FROM identity."user" WHERE id=? AND is_delete AND status='disabled' AND revision=? AND session_epoch=? AND platform_epoch=?`, current.User.ID, current.User.Revision+3, current.User.SessionEpoch+2, current.User.PlatformEpoch+2) != 1 {
		t.Fatal("final native deletion lost original counters")
	}
	for _, action := range []string{"disable", "enable", "delete"} {
		if count(t, f.idb, `SELECT count(*) FROM identity.security_event WHERE target_user_id=? AND action=?`, current.User.ID, "account."+action) != 1 {
			t.Fatalf("native %s audit not exactly once", action)
		}
	}
	keys := []string{disableKey.String(), enableKey.String(), key.String(), deleteKey.String()}
	for _, id := range []uuid.UUID{disableKey, enableKey, key, deleteKey} {
		saved, err := pgidentity.NewSourceGovernanceStore(f.idb).LoadAcceptedGovernance(t.Context(), id)
		if err != nil || !saved.Completed {
			t.Fatalf("native original responsibility did not complete: %v", err)
		}
		controls := []writefence.Control{saved.CloseControl}
		if saved.ReopenControl.CoordinationID != uuid.Nil {
			controls = append(controls, saved.ReopenControl)
		}
		for _, control := range controls {
			state, err := pgfence.NewAuthorityStore(f.idb).LoadCoordination(t.Context(), control.CoordinationID)
			if err != nil || !state.Complete || state.Control != control || len(state.Acknowledged) != 4 {
				t.Fatalf("native terminal result preceded exact four ACKs: %v", err)
			}
		}
	}
	// A restart preserves original task leases. Let the real worker recover those
	// leases and complete every terminal responsibility without shortening them.
	deadline := time.NewTimer(35 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for count(t, f.idb, `SELECT count(*) FROM infra.task_work WHERE service_owner='identity' AND kind=? AND business_id IN ? AND completed_at IS NOT NULL`, identity.GovernanceRecoveryTask, keys) != 4 {
		select {
		case <-tick.C:
		case <-deadline.C:
			t.Fatal("native worker did not complete accepted and rejected original tasks")
		case <-t.Context().Done():
			t.Fatal(t.Context().Err())
		}
	}
}

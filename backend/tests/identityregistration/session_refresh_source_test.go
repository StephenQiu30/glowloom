package identityregistration_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/http"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/identity/app"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"sync/atomic"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/identity/adapter/refreshredis"
	"github.com/StephenQiu30/glowloom-server/backend/identity/adapter/sessiontoken"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	coordinationpb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/coordination"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	fencerpc "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/grpctransport"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

func configureSourceSessionSecurity(t *testing.T, f *registrationFixture) identityapp.SessionConfig {
	t.Helper()
	if os.Getenv("GLOWLOOM_TEST_SESSION_SOURCE_REDIS") != "owner-b-dedicated" {
		t.Skip("native session source real-Redis acceptance needs its dedicated loopback fixture")
	}
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:52081", Password: "glowloom-owner-b-synthetic-fixture", DialTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second})
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatal("dedicated source Redis fixture unavailable")
	}
	coordinator, err := refreshredis.New(client)
	if err != nil {
		t.Fatal(err)
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	private := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	defer clear(private)
	public := pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: x509.MarshalPKCS1PublicKey(&key.PublicKey)})
	codec, err := sessiontoken.New("native-source-fixture", private, map[string][]byte{"native-source-fixture": public})
	if err != nil {
		t.Fatal(err)
	}
	receiptKey := make([]byte, 32)
	if _, err := rand.Read(receiptKey); err != nil {
		t.Fatal(err)
	}
	receiptValue := base64.StdEncoding.EncodeToString(receiptKey)
	protector, err := sessiontoken.NewReceiptProtector(receiptKey)
	clear(receiptKey)
	if err != nil {
		t.Fatal(err)
	}
	f.auth.WithSessionSecurity(codec, protector, coordinator).WithSessionIssuance(true)
	f.auth.WithLegacyUpgradeUntil(f.clock.Add(time.Hour))
	store := pgidentity.NewSessionRevocationStore(f.idb)
	clock := func() time.Time { return f.clock }
	requests := identity.NewPublicSessionRevocations(f.auth, store, identity.NewSessionRevocations(store, f.writers, clock), clock)
	if err := f.auth.WithSessionRevocationRequests(requests); err != nil {
		t.Fatal(err)
	}
	verify, err := json.Marshal(map[string]string{"native-source-fixture": base64.StdEncoding.EncodeToString(public)})
	if err != nil {
		t.Fatal(err)
	}
	return identityapp.SessionConfig{Enabled: true, PrivateKey: base64.StdEncoding.EncodeToString(private), VerifyKeys: string(verify), ActiveKID: "native-source-fixture", ReceiptKey: receiptValue, RedisAddress: "127.0.0.1:52081", RedisPassword: client.Options().Password, LegacyUpgradeUntil: f.clock.Add(time.Hour)}
}

func sourceV2Fixture(t *testing.T) (*registrationFixture, identity.AuthResult) {
	t.Helper()
	f := newRegistrationFixture(t)
	input := f.input(t)
	id, err := f.auth.CompleteRegistration(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	var email string
	if err := f.idb.Raw(`SELECT email_key FROM identity."user" WHERE id=?`, id).Scan(&email).Error; err != nil {
		t.Fatal(err)
	}
	configureSourceSessionSecurity(t, f)
	if err := f.auth.WithSessionActivation(f.store, f.writers, f.protector); err != nil {
		t.Fatal(err)
	}
	result, err := f.auth.LoginFromCommand(t.Context(), uuid.New(), email, input.Password, false, "source-v2-"+uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	if result.AccessToken == "" || result.RefreshToken == "" || result.Session.ProtocolVersion != 2 {
		t.Fatal("real source login did not issue v2 credentials")
	}
	return f, result
}

func TestRefreshReusePersistsSourceClosureBeforeReturningInvalid(t *testing.T) {
	f, original := sourceV2Fixture(t)
	if _, err := f.auth.Refresh(t.Context(), original.RefreshToken, original.CSRFToken, "native-reuse-fixture", uuid.New()); err != nil {
		t.Fatal(err)
	}
	f.clock = f.clock.Add(11 * time.Second)
	f.controlLoss.kind = writefence.Session
	f.controlLoss.lose.Store(true)
	request := uuid.New()
	_, err := f.auth.Refresh(t.Context(), original.RefreshToken, original.CSRFToken, "native-reuse-fixture", request)
	if err == nil {
		t.Fatal("expired parent was accepted")
	}
	var head struct {
		Blocked, Complete bool
		Epoch             int64
	}
	if err := f.idb.Raw(`SELECT blocked,complete,epoch FROM infra.authority_head WHERE service_owner=infra.runtime_owner() AND issuer='identity' AND authority_kind='session' AND authority_id=?`, original.Session.ID).Scan(&head).Error; err != nil {
		t.Fatal(err)
	}
	if !head.Blocked || head.Complete || head.Epoch != 2 || errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatalf("reuse returned final invalid before original writer closure: head=%+v err=%v", head, err)
	}
	if _, err := f.auth.Refresh(t.Context(), original.RefreshToken, original.CSRFToken, "native-reuse-fixture", request); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatalf("original revoked/expired proof did not finish source closure: %v", err)
	}
	if count(t, f.idb, `SELECT count(*) FROM identity.security_event WHERE target_user_id=? AND action='session.revoked' AND reason='refresh reuse' AND request_id=?`, original.User.ID, request.String()) != 1 {
		t.Fatal("source reuse lost its original one-time audit reason/request")
	}
	store := pgidentity.NewSessionRevocationStore(f.idb)
	closed, err := store.LoadSessionRevocation(t.Context(), original.Session.ID)
	if err != nil || closed.CompletedAt == nil {
		t.Fatal("reuse source closure was not complete")
	}
	state, err := pgfence.NewAuthorityStore(f.idb).LoadCoordination(t.Context(), closed.Control.CoordinationID)
	if err != nil || !state.Complete || state.Control != closed.Control || len(state.Acknowledged) != 4 {
		t.Fatal("reuse lacked exact four-writer source receipts")
	}
	if _, err := f.auth.ReadAccessIdentity(t.Context(), original.AccessToken); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatalf("original real JWT survived source reuse closure: %v", err)
	}
	if count(t, f.idb, `SELECT count(*) FROM identity.session_revocation_request WHERE idem_key=? AND action='refresh_reuse' AND target_session_id=request_session_id AND completed_at IS NOT NULL`, request) != 1 {
		t.Fatal("reuse original request was not durably completed")
	}
}

func TestNativeRefreshAndUpgradeCannotPublishThroughPendingHeads(t *testing.T) {
	for _, upgrade := range []bool{false, true} {
		for _, kind := range []writefence.Kind{writefence.Account, writefence.Session} {
			t.Run(fmt.Sprintf("upgrade_%t_%s", upgrade, kind), func(t *testing.T) {
				var f *registrationFixture
				var result identity.AuthResult
				if upgrade {
					var input identity.PasswordChangeInput
					f, input = passwordFixture(t)
					var err error
					result, err = f.auth.Current(t.Context(), input.Credential.LegacyToken)
					if err != nil {
						t.Fatal(err)
					}
					result.Token, result.CSRFToken = input.Credential.LegacyToken, input.Credential.CSRFToken
					configureSourceSessionSecurity(t, f)
				} else {
					f, result = sourceV2Fixture(t)
				}
				id := result.User.ID
				if kind == writefence.Session {
					id = result.Session.ID
				}
				f.controlLoss.kind = kind
				f.controlLoss.lose.Store(true)
				if _, err := f.writers.Set(t.Context(), writefence.Authority{Issuer: writefence.Identity, Kind: kind, ID: id, Epoch: 1}, uuid.New(), true); err == nil {
					t.Fatal("pending close was not exercised")
				}
				var next identity.AuthResult
				var err error
				if upgrade {
					next, err = f.auth.UpgradeLegacy(t.Context(), result.Token, result.CSRFToken, "source-upgrade-fixture", uuid.New())
				} else {
					next, err = f.auth.Refresh(t.Context(), result.RefreshToken, result.CSRFToken, "source-refresh-fixture", uuid.New())
				}
				if err == nil || next.AccessToken != "" || next.RefreshToken != "" {
					t.Fatal("pending source head published new access/refresh credentials")
				}
				if count(t, f.idb, `SELECT count(*) FROM identity.user_session WHERE id=? AND refresh_generation=0 AND protocol_version=?`, result.Session.ID, result.Session.ProtocolVersion) != 1 {
					t.Fatal("failed source guard changed protocol/refresh generation")
				}
			})
		}
	}
}

func TestRefreshReusePurposeRequiresExpiredConsumedParentInPolicyTx(t *testing.T) {
	f, original := sourceV2Fixture(t)
	raw, err := base64.RawURLEncoding.DecodeString(original.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	clear(raw)
	csrf := sha256.Sum256([]byte(original.CSRFToken))
	request := identity.SessionRevocationInput{Key: uuid.New(), Action: identity.RevokeRefreshReuseRequest, TargetID: original.Session.ID, Primary: identity.SessionCredentialRefresh, Proofs: []identity.SessionRevocationProof{{Kind: identity.SessionCredentialRefresh, TokenDigest: digest[:], CSRFHash: csrf[:]}}}
	actor := identity.Principal{ID: original.User.ID, SessionID: original.Session.ID, CredentialEpoch: original.User.SessionEpoch}
	if _, err := pgidentity.NewSessionRevocationStore(f.idb).PrepareSessionRevocationRequest(t.Context(), actor, request, func() time.Time { return f.clock }); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatalf("unconsumed refresh entered internal reuse purpose: %v", err)
	}
	if count(t, f.idb, `SELECT count(*) FROM identity.session_revocation_request WHERE idem_key=?`, request.Key) != 0 {
		t.Fatal("invalid reuse enrolled a responsibility")
	}
	for _, target := range []any{nil, uuid.New()} {
		err := f.idb.Exec(`INSERT INTO identity.session_revocation_request(id,idem_key,account_id,request_session_id,action,target_session_id,requested_at) VALUES(?,?,?,?,'refresh_reuse',?,statement_timestamp())`, uuid.New(), uuid.New(), original.User.ID, original.Session.ID, target).Error
		var state interface{ SQLState() string }
		if !errors.As(err, &state) || state.SQLState() != "23514" {
			t.Fatalf("reuse target null/self constraint was not enforced: %v", err)
		}
	}
}

func TestIdentityRuntimeV2RefreshRecoversOriginalReuseAfterRestart(t *testing.T) {
	f := newRegistrationFixture(t)
	input := f.input(t)
	accountID, err := f.auth.CompleteRegistration(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	var email string
	if err := f.idb.Raw(`SELECT email_key FROM identity."user" WHERE id=?`, accountID).Scan(&email).Error; err != nil {
		t.Fatal(err)
	}
	cfg, _ := runtimeConfig(t, f)
	cfg.Session = configureSourceSessionSecurity(t, f)
	media := cfg.Writers[writefence.WriterMedia]
	handler, err := fencerpc.NewServer(writefence.WriterMedia, pgfence.New(registrationDB(t, "media")), media.Token, strings.Repeat("creation-writer-", 3)+"media")
	if err != nil {
		t.Fatal(err)
	}
	var lose atomic.Bool
	writer := grpc.NewServer(grpc.Creds(f.tls.server(t, media.ServerName)), grpc.UnaryInterceptor(func(ctx context.Context, request any, _ *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		response, err := next(ctx, request)
		if err == nil && lose.Load() {
			return nil, status.Error(codes.Unavailable, "synthetic committed writer ACK lost")
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
				t.Error("native v2 runtime did not join owned workers")
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
	send := func(path string, key uuid.UUID, body any, cookies []*http.Cookie, csrf string) *http.Response {
		var reader io.Reader
		if body != nil {
			encoded, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			reader = bytes.NewReader(encoded)
		}
		request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+address+path, reader)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Origin", cfg.PublicOrigin)
		request.Header.Set("Idempotency-Key", key.String())
		if body != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		if csrf != "" {
			request.Header.Set("X-CSRF-Token", csrf)
		}
		for _, cookie := range cookies {
			request.AddCookie(cookie)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	login := send("/api/sessions", uuid.New(), identityhttp.EmailLoginRequest{Email: email, Password: input.Password}, nil, "")
	if login.StatusCode != 201 {
		t.Fatalf("native v2 login=%d", login.StatusCode)
	}
	var session identityhttp.EmailSessionView
	if err := json.NewDecoder(login.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	_ = login.Body.Close()
	originalCookies := login.Cookies()
	if len(originalCookies) != 3 || session.CSRFToken == "" {
		t.Fatal("native v2 omitted access/refresh/CSRF cookie contract")
	}
	rotated := send("/api/session/refresh", uuid.New(), nil, originalCookies, session.CSRFToken)
	_ = rotated.Body.Close()
	if rotated.StatusCode != 200 {
		t.Fatalf("native v2 rotation=%d", rotated.StatusCode)
	}
	// The canonical receipt window is exactly ten seconds. Let the real runtime
	// and its cleanup worker cross that deadline without changing persisted facts.
	expiry := time.NewTimer(11 * time.Second)
	defer expiry.Stop()
	select {
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	case <-expiry.C:
	}
	lose.Store(true)
	key := uuid.New()
	unknown := send("/api/session/refresh", key, nil, originalCookies, session.CSRFToken)
	_ = unknown.Body.Close()
	if unknown.StatusCode != 503 || len(unknown.Cookies()) != 0 {
		t.Fatalf("unknown native reuse falsely finalized/minted cookies: %d", unknown.StatusCode)
	}
	stop()
	stop = nil
	lose.Store(false)
	start()
	replayed := send("/api/session/refresh", key, nil, originalCookies, session.CSRFToken)
	var problem struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(replayed.Body).Decode(&problem); err != nil {
		t.Fatal(err)
	}
	_ = replayed.Body.Close()
	if replayed.StatusCode != 401 || problem.Code != "session_invalid" || len(replayed.Cookies()) != 0 {
		t.Fatalf("native original reuse recovery=%d code=%s", replayed.StatusCode, problem.Code)
	}
	closed, err := pgidentity.NewSessionRevocationStore(f.idb).LoadSessionRevocation(t.Context(), session.SessionID)
	if err != nil || closed.CompletedAt == nil {
		t.Fatal("restarted native reuse source incomplete")
	}
	state, err := pgfence.NewAuthorityStore(f.idb).LoadCoordination(t.Context(), closed.Control.CoordinationID)
	if err != nil || !state.Complete || len(state.Acknowledged) != 4 || state.Control != closed.Control {
		t.Fatal("native restart lacked exact original four-writer receipts")
	}
	if count(t, f.idb, `SELECT count(*) FROM identity.security_event WHERE target_user_id=? AND action='session.revoked' AND reason='refresh reuse' AND request_id=?`, accountID, key.String()) != 1 {
		t.Fatal("native reuse restart repeated/lost original audit")
	}
	deadline := time.NewTimer(35 * time.Second)
	defer deadline.Stop()
	poll := time.NewTicker(50 * time.Millisecond)
	defer poll.Stop()
	for count(t, f.idb, `SELECT count(*) FROM infra.task_work t JOIN identity.session_revocation_request r ON r.id::text=t.business_id WHERE t.service_owner=infra.runtime_owner() AND t.kind=? AND r.idem_key=? AND t.completed_at IS NOT NULL`, identity.SessionRevocationRecoveryTask, key) != 1 {
		select {
		case <-t.Context().Done():
			t.Fatal(t.Context().Err())
		case <-deadline.C:
			t.Fatal("restarted native worker did not complete the original reuse task")
		case <-poll.C:
		}
	}
}

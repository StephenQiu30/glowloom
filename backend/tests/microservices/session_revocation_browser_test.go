package microservices_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/StephenQiu30/glowloom-server/backend/identity/adapter/credentialproof"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/http"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	fencerpc "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/grpctransport"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

func TestPublicSessionRevocationBrowserResumesAfterCredentialCleanup(t *testing.T) {
	for _, mode := range []string{"legacy-self", "access-self", "refresh-self", "access-others"} {
		t.Run(mode, func(t *testing.T) { testBrowserRevocation(t, mode) })
	}
}

func testBrowserRevocation(t *testing.T, mode string) {
	t.Helper()
	admin := fenceDatabase(t, "")
	database := authorityDatabase(t, "identity")
	now := time.Now().UTC().Truncate(time.Microsecond)
	actor, other := sessionRevocationActor(t, admin, now)
	proof, err := credentialproof.New("browser-fixture", bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32))
	if err != nil {
		t.Fatal(err)
	}
	opaque := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32))
	refresh := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{4}, 32))
	csrf := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{5}, 32))
	access := "fixture-access-" + actor.SessionID.String()
	if mode == "legacy-self" {
		csrf = base64.RawURLEncoding.EncodeToString(proof.Digest("session_csrf", actor.SessionID, opaque))
	}
	csrfHash := sha256.Sum256([]byte(csrf))
	raw, _ := base64.RawURLEncoding.DecodeString(opaque)
	tokenHash := sha256.Sum256(raw)
	protocol := 2
	if mode == "legacy-self" {
		protocol = 1
	}
	if err := admin.Exec(`UPDATE identity.user_session SET token_hash=?,csrf_hash=?,protocol_version=? WHERE id=?`, tokenHash[:], csrfHash[:], protocol, actor.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	rawRefresh, _ := base64.RawURLEncoding.DecodeString(refresh)
	refreshHash := sha256.Sum256(rawRefresh)
	if err := admin.Exec(`UPDATE identity.session_refresh_token SET token_digest=? WHERE session_id=?`, refreshHash[:], actor.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	codec := &browserAccessCodec{token: access, claims: identity.AccessClaims{UserID: actor.ID, SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch, IssuedAt: now, ExpiresAt: now.Add(time.Hour)}}
	auth := identity.NewEmailAuth(pgidentity.NewStore(admin), proof, identity.EmailPolicy{TermsVersion: "test"}, func() time.Time { return now }).WithLegacyUpgradeUntil(now.Add(time.Hour)).WithSessionSecurity(codec, nil, &browserRefreshHints{})
	targets := make(map[writefence.Writer]fencerpc.Target)
	calls := make(map[writefence.Writer]*atomic.Int32)
	var loseACK atomic.Bool
	loseACK.Store(true)
	for _, writer := range writefence.Writers() {
		server, err := fencerpc.NewServer(writer, pgfence.New(authorityDatabase(t, string(writer))), identityFenceToken(writer), creationFenceToken(writer))
		if err != nil {
			t.Fatal(err)
		}
		counter := new(atomic.Int32)
		calls[writer] = counter
		address := startFenceRPC(t, server, func(ctx context.Context, request any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			counter.Add(1)
			response, err := handler(ctx, request)
			if err == nil && writer == writefence.WriterMedia && loseACK.CompareAndSwap(true, false) {
				return nil, status.Error(codes.Unavailable, "synthetic acknowledgement loss")
			}
			return response, err
		})
		targets[writer] = fencerpc.Target{Address: address, Token: identityFenceToken(writer), Transport: insecure.NewCredentials()}
	}
	buildRouter := func() *gin.Engine {
		client, err := fencerpc.NewClient(writefence.Identity, targets)
		if err != nil {
			t.Fatal(err)
		}
		store := pgidentity.NewSessionRevocationStore(database)
		coordinator := writefence.NewCoordinator(pgfence.NewAuthorityStore(database), client)
		service := identity.NewPublicSessionRevocations(auth, store, identity.NewSessionRevocations(store, coordinator, func() time.Time { return now }), func() time.Time { return now })
		router := gin.New()
		identityhttp.NewEmailAuthHandler(auth, "https://public.example", false).WithPublicSessionRevocations(service).Register(router.Group("/api"))
		return router
	}
	key := uuid.NewString()
	path := "/api/sessions/" + actor.SessionID.String()
	method := http.MethodDelete
	if mode == "access-others" {
		path = "/api/me/sessions/revoke-others"
		method = http.MethodPost
	}
	cookie := "glowloom_access=" + access + "; glowloom_refresh=" + refresh + "; glowloom_csrf=" + csrf
	if mode == "legacy-self" {
		cookie = "glowloom_session=" + opaque
	}
	if mode == "refresh-self" {
		cookie = "glowloom_refresh=" + refresh + "; glowloom_csrf=" + csrf
	}
	invoke := func(router *gin.Engine, key, path, cookie, csrf string) *httptest.ResponseRecorder {
		request := httptest.NewRequestWithContext(t.Context(), method, path, nil)
		request.Header.Set("Origin", "https://public.example")
		request.Header.Set("Idempotency-Key", key)
		request.Header.Set("Cookie", cookie)
		request.Header.Set("X-CSRF-Token", csrf)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	first := invoke(buildRouter(), key, path, cookie, csrf)
	if first.Code != 503 || len(first.Result().Cookies()) != 0 {
		t.Fatalf("unconfirmed first response=%d cookies=%d body=%s", first.Code, len(first.Result().Cookies()), first.Body.String())
	}
	target := actor.SessionID
	if mode == "access-others" {
		target = other
	}
	assertSessionLocallyRevoked(t, admin, target)
	// Remove the original authenticating credential rows. Recovery must use only
	// the accepted request and its proof, not CurrentAccess, a refresh hint or UUID.
	if err := admin.Exec(`DELETE FROM identity.session_refresh_token WHERE session_id=?`, actor.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if err := admin.Exec(`DELETE FROM identity.user_session WHERE id=?`, actor.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	codec.expired = true
	if mode == "access-self" || mode == "access-others" {
		cookie = "glowloom_refresh=" + refresh + "; glowloom_csrf=" + csrf
	}
	before := calls[writefence.WriterIdentity].Load()
	rejected := invoke(buildRouter(), key, path, cookie, base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32)))
	if rejected.Code != 403 || calls[writefence.WriterIdentity].Load() != before {
		t.Fatalf("wrong proof recovered original request: %d", rejected.Code)
	}
	if mode != "access-others" {
		retarget := invoke(buildRouter(), key, "/api/sessions/"+other.String(), cookie, csrf)
		if retarget.Code != 403 {
			t.Fatalf("retargeted original proof status=%d body=%s", retarget.Code, retarget.Body.String())
		}
	}
	second := invoke(buildRouter(), key, path, cookie, csrf)
	if second.Code != 204 {
		t.Fatalf("original request could not recover after cleanup: status=%d body=%s", second.Code, second.Body.String())
	}
	if mode == "access-others" {
		if len(second.Result().Cookies()) != 0 {
			t.Fatal("batch cleared the requester's cookies")
		}
	} else if len(second.Result().Cookies()) == 0 {
		t.Fatal("completed self logout did not clear cookies")
	}
	for _, writer := range writefence.Writers() {
		want := int32(1)
		if writer == writefence.WriterMedia {
			want = 2
		}
		if got := calls[writer].Load(); got != want {
			t.Fatalf("writer=%s calls=%d want=%d", writer, got, want)
		}
	}
	if third := invoke(buildRouter(), key, path, cookie, csrf); third.Code != 204 {
		t.Fatalf("durable completed retry status=%d", third.Code)
	}
	fresh := invoke(buildRouter(), uuid.NewString(), path, cookie, csrf)
	if fresh.Code == 204 {
		t.Fatal("old proof authorized a new request after credential cleanup")
	}
}

type browserAccessCodec struct {
	token   string
	claims  identity.AccessClaims
	expired bool
}

func (*browserAccessCodec) CanSign() bool { return false }
func (*browserAccessCodec) Sign(identity.AccessClaims) (string, error) {
	return "", errors.New("fixture cannot sign")
}
func (c *browserAccessCodec) Verify(token string, _ time.Time) (identity.AccessClaims, error) {
	if c.expired {
		return identity.AccessClaims{}, identity.ErrAccessExpired
	}
	if token != c.token {
		return identity.AccessClaims{}, identity.ErrAccessInvalid
	}
	return c.claims, nil
}

type browserRefreshHints struct{ identity.RefreshCoordinator }

func (*browserRefreshHints) IsRevoked(context.Context, uuid.UUID) (bool, error) { return false, nil }

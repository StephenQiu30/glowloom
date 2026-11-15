package app_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/identityruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

func TestBusinessRouterConsumesDurableRevocationWithoutIssuanceOrCurrentSession(t *testing.T) {
	for _, batch := range []bool{false, true} {
		t.Run(map[bool]string{false: "single", true: "others"}[batch], func(t *testing.T) {
			now := time.Now().UTC()
			id, account, session, target := uuid.New(), uuid.New(), uuid.New(), uuid.New()
			request := identity.SessionRevocationRequest{ID: uuid.New(), Key: id, AccountID: account, RequestSessionID: session, Action: identity.RevokeSessionRequest, TargetID: target, RequestedAt: now, CompletedAt: &now, Revocations: []identity.SessionRevocation{{AccountID: account, SessionID: target, RequestSessionID: session, Control: writefence.Control{Authority: writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Session, ID: target, Epoch: 1}, Blocked: true, CoordinationID: uuid.New()}, RequestedAt: now, CompletedAt: &now}}}
			method, path := http.MethodDelete, "/api/sessions/"+target.String()
			if batch {
				request.Action = identity.RevokeOtherSessionsRequest
				request.TargetID = uuid.Nil
				method = http.MethodPost
				path = "/api/me/sessions/revoke-others"
			}
			store := &routedOriginalRevocation{request: request}
			cfg := config.Config{ObjectStorage: businessUnitStorage(t), Env: "local", HTTPAddr: "127.0.0.1:8080", PublicOrigin: "http://localhost:3000", IdentityKey: publicTestKey(t)}
			dependencies := identityruntime.SessionDependencies{Revocations: &identityruntime.SessionRevocationDependencies{Requests: store, Closures: &unusedSourceClosures{}, Writers: &unusedRevocationWriters{}}}
			router, err := app.NewBusinessRouter(zap.NewNop(), nil, noop.NewTracerProvider(), cfg, &gorm.DB{}, businessUnitRegistry(t), dependencies)
			if err != nil {
				t.Fatal(err)
			}
			csrf := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32))
			legacy := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{4}, 32))
			input := httptest.NewRequestWithContext(t.Context(), method, path, nil)
			input.Header.Set("Origin", cfg.PublicOrigin)
			input.Header.Set("Idempotency-Key", id.String())
			input.Header.Set("X-CSRF-Token", csrf)
			input.AddCookie(&http.Cookie{Name: "glowloom_session", Value: legacy})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, input)
			if response.Code != 204 || store.calls != 1 || len(response.Result().Cookies()) != 0 {
				t.Fatalf("router bypassed original recovery or cleared other-device cookies: status=%d calls=%d body=%s", response.Code, store.calls, response.Body.String())
			}
			// Ordinary listing still goes through RequireSession and cannot use this proof.
			listing := httptest.NewRecorder()
			router.ServeHTTP(listing, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/me/sessions", nil))
			if listing.Code != 401 || store.calls != 1 {
				t.Fatalf("listing inherited recovery authority: %d", listing.Code)
			}
		})
	}
}

func TestBusinessRouterRejectsPartialDurableRevocation(t *testing.T) {
	if service, err := identityruntime.NewPublicSessionRevocations(nil, nil, time.Now); err != nil || service != nil {
		t.Fatalf("unswitched owner unexpectedly enabled: %v", err)
	}
	auth := identity.NewEmailAuth(nil, nil, identity.EmailPolicy{}, time.Now)
	for _, deps := range []*identityruntime.SessionRevocationDependencies{
		{}, {Requests: &routedOriginalRevocation{}}, {Closures: &unusedSourceClosures{}}, {Writers: &unusedRevocationWriters{}},
	} {
		if _, err := identityruntime.NewPublicSessionRevocations(auth, deps, time.Now); err == nil {
			t.Fatal("partial durable owner fell back to local success")
		}
	}
}

type routedOriginalRevocation struct {
	identity.SessionRevocationRequestStore
	request identity.SessionRevocationRequest
	calls   int
}

func (s *routedOriginalRevocation) LoadSessionRevocationRequest(_ context.Context, input identity.SessionRevocationInput) (identity.SessionRevocationRequest, error) {
	s.calls++
	if input.Key != s.request.Key || input.Action != s.request.Action || input.TargetID != s.request.TargetID {
		return identity.SessionRevocationRequest{}, identity.ErrForbidden
	}
	return s.request, nil
}

type unusedSourceClosures struct {
	identity.SessionRevocationStore
}
type unusedRevocationWriters struct {
	identity.SessionFenceCoordinator
}

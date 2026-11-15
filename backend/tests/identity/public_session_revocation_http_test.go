package identity_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/http"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
)

func TestPublicSessionRevocationHTTPRequiresOriginalTransportEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, origin, csrf, key, cookie string
		status, calls                                 int
	}{
		{"legacy", http.MethodDelete, "/sessions/", "https://public.example", "original-csrf", "key", "glowloom_session=original-legacy", 204, 1},
		{"modern", http.MethodDelete, "/sessions/", "https://public.example", "original-csrf", "key", "glowloom_access=access; glowloom_refresh=refresh; glowloom_csrf=original-csrf", 204, 1},
		{"modern refresh recovery", http.MethodDelete, "/sessions/", "https://public.example", "original-csrf", "key", "glowloom_refresh=refresh; glowloom_csrf=original-csrf", 204, 1},
		{"batch", http.MethodPost, "/me/sessions/revoke-others", "https://public.example", "original-csrf", "key", "glowloom_session=original-legacy", 204, 1},
		{"wrong origin", http.MethodDelete, "/sessions/", "https://wrong.example", "original-csrf", "key", "glowloom_session=original-legacy", 403, 0},
		{"missing key", http.MethodDelete, "/sessions/", "https://public.example", "original-csrf", "", "glowloom_session=original-legacy", 422, 0},
		{"duplicate cookie", http.MethodDelete, "/sessions/", "https://public.example", "original-csrf", "key", "glowloom_session=one; glowloom_session=two", 401, 0},
		{"mixed protocol", http.MethodDelete, "/sessions/", "https://public.example", "original-csrf", "key", "glowloom_session=legacy; glowloom_access=access; glowloom_csrf=original-csrf", 401, 0},
		{"modern csrf mismatch", http.MethodDelete, "/sessions/", "https://public.example", "wrong-csrf", "key", "glowloom_access=access; glowloom_csrf=original-csrf", 403, 0},
		{"missing csrf", http.MethodDelete, "/sessions/", "https://public.example", "", "key", "glowloom_session=legacy", 403, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			commands := &publicRevocationHTTPModel{clear: true}
			router := gin.New()
			identityhttp.NewEmailAuthHandler(nil, "https://public.example", false).WithPublicSessionRevocations(commands).Register(router.Group("/api"))
			path := tc.path
			if tc.method == http.MethodDelete {
				path += uuid.NewString()
			}
			request := httptest.NewRequestWithContext(t.Context(), tc.method, "/api"+path, nil)
			request.Header.Set("Origin", tc.origin)
			request.Header.Set("X-CSRF-Token", tc.csrf)
			request.Header.Set("Cookie", tc.cookie)
			if tc.key != "" {
				request.Header.Set("Idempotency-Key", uuid.NewString())
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.status || commands.calls != tc.calls {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, commands.calls, response.Body.String())
			}
			if (tc.status != 204 || tc.method == http.MethodPost) && len(response.Result().Cookies()) != 0 {
				t.Fatal("failed or other-device request cleared cookies")
			}
			if tc.status == 204 && tc.method == http.MethodDelete && len(response.Result().Cookies()) == 0 {
				t.Fatal("confirmed self revocation retained credentials")
			}
		})
	}
}

func TestPublicSessionRevocationHTTPDoesNotClearCookiesBeforeCompletion(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		clear  bool
		status int
	}{
		{"pending writer", errors.New("synthetic unavailable writer"), false, 503},
		{"proof mismatch", identity.ErrSessionCSRFInvalid, false, 403},
		{"retarget", identity.ErrForbidden, false, 403},
		{"other device", nil, false, 204},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := &publicRevocationHTTPModel{err: tc.err, clear: tc.clear}
			router := gin.New()
			identityhttp.NewEmailAuthHandler(nil, "https://public.example", false).WithPublicSessionRevocations(model).Register(router.Group("/api"))
			request := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/api/sessions/"+uuid.NewString(), nil)
			request.Header.Set("Origin", "https://public.example")
			request.Header.Set("Idempotency-Key", uuid.NewString())
			request.Header.Set("X-CSRF-Token", "original-csrf")
			request.Header.Set("Cookie", "glowloom_session=legacy")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.status || len(response.Result().Cookies()) != 0 {
				t.Fatalf("unconfirmed self result status=%d cookies=%d", response.Code, len(response.Result().Cookies()))
			}
		})
	}
}

type publicRevocationHTTPModel struct {
	calls int
	err   error
	clear bool
}

func (m *publicRevocationHTTPModel) Revoke(_ context.Context, credentials identity.SessionRevocationCredentials, key, target uuid.UUID) (bool, error) {
	m.calls++
	if key == uuid.Nil || target == uuid.Nil || credentials.CSRFToken == "" {
		return false, errors.New("missing immutable request")
	}
	return m.clear, m.err
}
func (m *publicRevocationHTTPModel) RevokeOthers(_ context.Context, credentials identity.SessionRevocationCredentials, key uuid.UUID) error {
	m.calls++
	if key == uuid.Nil || credentials.CSRFToken == "" {
		return errors.New("missing immutable batch")
	}
	return m.err
}

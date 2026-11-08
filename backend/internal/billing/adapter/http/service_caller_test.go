package http

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"
)

type creditNonceStoreStub struct{ claimed map[string]bool }

func (s *creditNonceStoreStub) Claim(_ context.Context, callerID, keyID, nonce string, _ time.Duration) (bool, error) {
	key := callerID + "/" + keyID + "/" + nonce
	if s.claimed[key] {
		return false, nil
	}
	s.claimed[key] = true
	return true, nil
}

type creditCommandServiceStub struct{ submitted bool }

func (s *creditCommandServiceStub) Submit(_ context.Context, _ application.CreditCommand, _ string) (application.CreditCommandReceipt, error) {
	s.submitted = true
	return application.CreditCommandReceipt{}, nil
}

type walletReadLimiterStub struct{ calls int }

func (s *walletReadLimiterStub) AllowWalletRead(context.Context, string, uuid.UUID, string, time.Time) (time.Duration, error) {
	s.calls++
	return 0, nil
}

func (s *creditCommandServiceStub) Read(context.Context, string, uuid.UUID, uuid.UUID) (application.CreditCommandReceipt, error) {
	return application.CreditCommandReceipt{}, nil
}

func TestCreditServiceAuthenticatorSignsBodyAndRejectsReplay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, 11, 8, 3, 4, 5, 0, time.UTC)
	secret := strings.Repeat("k", 40)
	scopeID := uuid.New()
	nonces := &creditNonceStoreStub{claimed: make(map[string]bool)}
	auth, err := NewCreditServiceAuthenticator([]CreditServiceCaller{{
		ID: "short-drama", Scopes: []uuid.UUID{scopeID}, Actions: []application.CreditAction{application.CreditFreeze},
		Keys: []CreditServiceKey{{ID: "key-1", Secret: secret}},
	}}, nonces, func() time.Time { return now })
	if err != nil {
		t.Fatalf("configure service auth: %v", err)
	}
	engine := gin.New()
	engine.POST("/api/internal/billing/scopes/:scopeID/credit-commands", auth, func(c *gin.Context) {
		principal, ok := creditServicePrincipalFrom(c)
		if !ok || principal.callerID != "short-drama" || !principal.allowsScope(scopeID) || !principal.allowsAction(application.CreditFreeze) {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Status(http.StatusNoContent)
	})
	path := "/api/internal/billing/scopes/" + scopeID.String() + "/credit-commands"
	nonce := "nonce_0123456789abcdefghijklmnop"
	request := signedCreditRequest(http.MethodPost, path, `{"action":"freeze"}`, "short-drama", "key-1", nonce, secret, now)
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("signed service request status = %d body %s, want 204", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, signedCreditRequest(http.MethodPost, path, `{"action":"freeze"}`, "short-drama", "key-1", nonce, secret, now))
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "service_auth_replayed") {
		t.Fatalf("replayed service request status = %d body %s, want replay rejection", response.Code, response.Body.String())
	}
}

func TestCreditServiceAuthenticatorRejectsTamperingAndExpiredTimestamp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, 11, 8, 3, 4, 5, 0, time.UTC)
	secret := strings.Repeat("k", 40)
	scopeID := uuid.New()
	for _, tc := range []struct {
		name       string
		signedBody string
		body       string
		timestamp  time.Time
		wantCode   string
	}{
		{name: "body tampered", signedBody: `{"action":"freeze"}`, body: `{"action":"release"}`, timestamp: now, wantCode: "service_auth_invalid"},
		{name: "timestamp expired", signedBody: `{"action":"freeze"}`, body: `{"action":"freeze"}`, timestamp: now.Add(-creditServiceClockSkew - time.Second), wantCode: "service_auth_expired"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nonces := &creditNonceStoreStub{claimed: make(map[string]bool)}
			auth, err := NewCreditServiceAuthenticator([]CreditServiceCaller{{
				ID: "short-drama", Scopes: []uuid.UUID{scopeID}, Actions: []application.CreditAction{application.CreditFreeze},
				Keys: []CreditServiceKey{{ID: "key-1", Secret: secret}},
			}}, nonces, func() time.Time { return now })
			if err != nil {
				t.Fatalf("configure service auth: %v", err)
			}
			engine := gin.New()
			engine.POST("/credit-commands", auth, func(c *gin.Context) { c.Status(http.StatusNoContent) })
			request := signedCreditRequest(http.MethodPost, "/credit-commands", tc.signedBody, "short-drama", "key-1", "nonce_0123456789abcdefghijklmnop", secret, tc.timestamp)
			if tc.body != tc.signedBody {
				request.Body = io.NopCloser(strings.NewReader(tc.body))
				request.ContentLength = int64(len(tc.body))
			}
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), tc.wantCode) {
				t.Fatalf("request status = %d body %s, want %s", response.Code, response.Body.String(), tc.wantCode)
			}
		})
	}
}

func TestCreditServiceCallerCannotGrantOutsideActionScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now().UTC()
	secret := strings.Repeat("k", 40)
	scopeID := uuid.New()
	nonces := &creditNonceStoreStub{claimed: make(map[string]bool)}
	auth, err := NewCreditServiceAuthenticator([]CreditServiceCaller{{
		ID: "short-drama", Scopes: []uuid.UUID{scopeID}, Actions: []application.CreditAction{application.CreditFreeze},
		Keys: []CreditServiceKey{{ID: "key-1", Secret: secret}},
	}}, nonces, func() time.Time { return now })
	if err != nil {
		t.Fatalf("configure service auth: %v", err)
	}
	service := &creditCommandServiceStub{}
	engine := gin.New()
	engine.POST("/api/internal/billing/scopes/:scopeID/credit-commands", auth, NewCreditCommandHandler(service, nil, nil).Submit)
	path := "/api/internal/billing/scopes/" + scopeID.String() + "/credit-commands"
	body := fmt.Sprintf(`{"action":"grant","source_id":%q,"amount_credits":1,"reason":"test"}`, uuid.NewString())
	request := signedCreditRequest(http.MethodPost, path, body, "short-drama", "key-1", "nonce_0123456789abcdefghijklmnop", secret, now)
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || service.submitted {
		t.Fatalf("unauthorized grant status=%d submitted=%t body=%s", response.Code, service.submitted, response.Body.String())
	}
}

func TestCreditServiceReadsOnlyMappedWalletScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now().UTC()
	secret := strings.Repeat("k", 40)
	scopeID := uuid.New()
	readLimiter := &walletReadLimiterStub{}
	readCalled := false
	auth, err := NewCreditServiceAuthenticator([]CreditServiceCaller{{
		ID: "short-drama", Scopes: []uuid.UUID{scopeID}, Actions: []application.CreditAction{application.CreditRead},
		Keys: []CreditServiceKey{{ID: "key-1", Secret: secret}},
	}}, &creditNonceStoreStub{claimed: make(map[string]bool)}, func() time.Time { return now })
	if err != nil {
		t.Fatalf("configure service auth: %v", err)
	}
	engine := gin.New()
	handler := NewCreditCommandHandler(nil, func(_ context.Context, scope uuid.UUID, _ *uuid.UUID) (application.WalletView, error) {
		readCalled = true
		if scope != scopeID {
			t.Fatalf("wallet reader received scope %s, want %s", scope, scopeID)
		}
		return application.WalletView{Enabled: true, GeneralCredits: "3", HistoryAvailable: true}, nil
	}, readLimiter)
	engine.GET("/api/internal/billing/scopes/:scopeID/wallet", auth, handler.ReadWallet)
	path := "/api/internal/billing/scopes/" + scopeID.String() + "/wallet"
	request := signedCreditRequest(http.MethodGet, path, "", "short-drama", "key-1", "nonce_0123456789abcdefghijklmnop", secret, now)
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !readCalled || readLimiter.calls != 1 || !strings.Contains(response.Body.String(), `"general_credits":"3"`) {
		t.Fatalf("wallet read status=%d called=%t limited=%d body=%s", response.Code, readCalled, readLimiter.calls, response.Body.String())
	}

	otherScope := uuid.New()
	otherPath := "/api/internal/billing/scopes/" + otherScope.String() + "/wallet"
	request = signedCreditRequest(http.MethodGet, otherPath, "", "short-drama", "key-1", "nonce_abcdefghijklmnopqrstuvwx012345", secret, now)
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || readLimiter.calls != 1 {
		t.Fatalf("unmapped wallet scope status=%d limiter_calls=%d body=%s", response.Code, readLimiter.calls, response.Body.String())
	}
}

func signedCreditRequest(method, path, body, callerID, keyID, nonce, secret string, at time.Time) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	timestamp := fmt.Sprintf("%d", at.Unix())
	bodyHash := sha256.Sum256([]byte(body))
	canonical := strings.Join([]string{method, path, hex.EncodeToString(bodyHash[:]), timestamp, nonce, callerID, keyID}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	request.Header.Set("X-Credit-Caller", callerID)
	request.Header.Set("X-Credit-Key-Id", keyID)
	request.Header.Set("X-Credit-Timestamp", timestamp)
	request.Header.Set("X-Credit-Nonce", nonce)
	request.Header.Set("X-Credit-Signature", hex.EncodeToString(mac.Sum(nil)))
	return request
}

func TestCreditServiceCallerKeyRotationAcceptsTwoConfiguredKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now().UTC()
	scopeID := uuid.New()
	keys := []CreditServiceKey{{ID: "old", Secret: strings.Repeat("o", 32)}, {ID: "new", Secret: strings.Repeat("n", 32)}}
	auth, err := NewCreditServiceAuthenticator([]CreditServiceCaller{{
		ID: "short-drama", Scopes: []uuid.UUID{scopeID}, Actions: []application.CreditAction{application.CreditRead}, Keys: keys,
	}}, &creditNonceStoreStub{claimed: make(map[string]bool)}, func() time.Time { return now })
	if err != nil {
		t.Fatalf("configure overlapping rotation keys: %v", err)
	}
	engine := gin.New()
	engine.GET("/wallet", auth, func(c *gin.Context) { c.Status(http.StatusNoContent) })
	for _, key := range keys {
		request := signedCreditRequest(http.MethodGet, "/wallet", "", "short-drama", key.ID, "nonce_"+key.ID+"_0123456789abcdefghijklmnop", key.Secret, now)
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Fatalf("key %q status = %d body %s", key.ID, response.Code, response.Body.String())
		}
	}
}

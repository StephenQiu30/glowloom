package http

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

const (
	creditServiceBodyLimit = 32 << 10
	creditServiceClockSkew = 5 * time.Minute
	creditServiceNonceTTL  = 10 * time.Minute
	creditServiceContext   = "billing.credit_service_principal"
)

// CreditServiceKey is one active or retiring HMAC key for a caller.
type CreditServiceKey struct {
	ID     string
	Secret string
}

// CreditServiceCaller restricts a service identity to explicit wallet scopes and commands.
type CreditServiceCaller struct {
	ID      string
	Scopes  []uuid.UUID
	Actions []application.CreditAction
	Keys    []CreditServiceKey
}

// CreditServiceNonceStore atomically claims a signed request nonce until its replay window expires.
type CreditServiceNonceStore interface {
	Claim(context.Context, string, string, string, time.Duration) (bool, error)
}

type creditServicePrincipal struct {
	callerID string
	scopes   map[uuid.UUID]struct{}
	actions  map[application.CreditAction]struct{}
}

type creditServiceKey struct {
	id     string
	secret []byte
}

type creditServiceCaller struct {
	id      string
	scopes  map[uuid.UUID]struct{}
	actions map[application.CreditAction]struct{}
	keys    map[string][]byte
}

// NewCreditServiceAuthenticator verifies signed service requests and rejects nonce replays.
func NewCreditServiceAuthenticator(callers []CreditServiceCaller, nonces CreditServiceNonceStore, clock func() time.Time) (gin.HandlerFunc, error) {
	if len(callers) == 0 || nonces == nil {
		return nil, fmt.Errorf("credit service authentication is not configured")
	}
	if clock == nil {
		clock = time.Now
	}
	configured := make(map[string]creditServiceCaller, len(callers))
	for _, caller := range callers {
		if strings.TrimSpace(caller.ID) != caller.ID || caller.ID == "" || len(caller.ID) > 80 || len(caller.Scopes) == 0 || len(caller.Actions) == 0 || len(caller.Keys) == 0 {
			return nil, fmt.Errorf("invalid credit service caller configuration")
		}
		if _, exists := configured[caller.ID]; exists {
			return nil, fmt.Errorf("duplicate credit service caller")
		}
		entry := creditServiceCaller{id: caller.ID, scopes: make(map[uuid.UUID]struct{}, len(caller.Scopes)), actions: make(map[application.CreditAction]struct{}, len(caller.Actions)), keys: make(map[string][]byte, len(caller.Keys))}
		for _, scope := range caller.Scopes {
			if scope == uuid.Nil {
				return nil, fmt.Errorf("invalid credit service scope")
			}
			entry.scopes[scope] = struct{}{}
		}
		for _, action := range caller.Actions {
			switch action {
			case application.CreditGrant, application.CreditDebit, application.CreditFreeze, application.CreditCommit, application.CreditRelease, application.CreditRead:
				entry.actions[action] = struct{}{}
			default:
				return nil, fmt.Errorf("invalid credit service action")
			}
		}
		for _, key := range caller.Keys {
			if key.ID == "" || len(key.ID) > 80 || len(key.Secret) < 32 || len(key.Secret) > 256 {
				return nil, fmt.Errorf("invalid credit service key")
			}
			if _, exists := entry.keys[key.ID]; exists {
				return nil, fmt.Errorf("duplicate credit service key")
			}
			entry.keys[key.ID] = []byte(key.Secret)
		}
		configured[caller.ID] = entry
	}

	return func(c *gin.Context) {
		principal, status, problem := authenticateCreditServiceRequest(c, configured, nonces, clock().UTC())
		if problem != "" {
			httpapi.WriteProblem(c, status, problem, nil)
			c.Abort()
			return
		}
		c.Set(creditServiceContext, principal)
		c.Next()
	}, nil
}

func authenticateCreditServiceRequest(c *gin.Context, callers map[string]creditServiceCaller, nonces CreditServiceNonceStore, now time.Time) (creditServicePrincipal, int, string) {
	callerID := c.GetHeader("X-Credit-Caller")
	keyID := c.GetHeader("X-Credit-Key-Id")
	timestamp := c.GetHeader("X-Credit-Timestamp")
	nonce := c.GetHeader("X-Credit-Nonce")
	signature := c.GetHeader("X-Credit-Signature")
	caller, ok := callers[callerID]
	secret, keyOK := caller.keys[keyID]
	if !ok || !keyOK || len(timestamp) == 0 || len(timestamp) > 12 || len(nonce) < 22 || len(nonce) > 128 || !validCreditNonce(nonce) || len(signature) != sha256.Size*2 {
		return creditServicePrincipal{}, http.StatusUnauthorized, "service_auth_invalid"
	}
	unix, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || strconv.FormatInt(unix, 10) != timestamp {
		return creditServicePrincipal{}, http.StatusUnauthorized, "service_auth_invalid"
	}
	requestTime := time.Unix(unix, 0).UTC()
	if requestTime.Before(now.Add(-creditServiceClockSkew)) || requestTime.After(now.Add(creditServiceClockSkew)) {
		return creditServicePrincipal{}, http.StatusUnauthorized, "service_auth_expired"
	}
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, creditServiceBodyLimit))
	if err != nil {
		return creditServicePrincipal{}, http.StatusRequestEntityTooLarge, "request_too_large"
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	bodyDigest := sha256.Sum256(body)
	requestURI := c.Request.URL.EscapedPath()
	if c.Request.URL.RawQuery != "" {
		requestURI += "?" + c.Request.URL.RawQuery
	}
	canonical := strings.Join([]string{
		c.Request.Method,
		requestURI,
		hex.EncodeToString(bodyDigest[:]),
		timestamp,
		nonce,
		callerID,
		keyID,
	}, "\n")
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(canonical))
	provided, err := hex.DecodeString(signature)
	if err != nil || !hmac.Equal(provided, mac.Sum(nil)) {
		return creditServicePrincipal{}, http.StatusUnauthorized, "service_auth_invalid"
	}
	claimed, err := nonces.Claim(c.Request.Context(), callerID, keyID, nonce, creditServiceNonceTTL)
	if err != nil {
		return creditServicePrincipal{}, http.StatusServiceUnavailable, "dependency_unavailable"
	}
	if !claimed {
		return creditServicePrincipal{}, http.StatusUnauthorized, "service_auth_replayed"
	}
	principal := creditServicePrincipal{callerID: caller.id, scopes: caller.scopes, actions: caller.actions}
	return principal, http.StatusOK, ""
}

func validCreditNonce(nonce string) bool {
	for _, r := range nonce {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func creditServicePrincipalFrom(c *gin.Context) (creditServicePrincipal, bool) {
	value, ok := c.Get(creditServiceContext)
	if !ok {
		return creditServicePrincipal{}, false
	}
	principal, ok := value.(creditServicePrincipal)
	return principal, ok
}

func (p creditServicePrincipal) allowsScope(scopeID uuid.UUID) bool {
	_, ok := p.scopes[scopeID]
	return ok
}

func (p creditServicePrincipal) allowsAction(action application.CreditAction) bool {
	_, ok := p.actions[action]
	return ok
}

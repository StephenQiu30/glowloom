package httpapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	platformhttp "github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	app "github.com/StephenQiu30/lanverse/backend/internal/production/creation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/creation/domain"
)

const sourceAuthorizationHeader = "X-Lanverse-Creation-Authorization"

type FrozenSourceService interface {
	ReadSource(context.Context, string, string) (domain.FrozenSource, error)
}

type SourceHandler struct {
	service FrozenSourceService
	secret  []byte
	now     func() time.Time
}

func NewSourceHandler(service FrozenSourceService, secret string, now func() time.Time) (*SourceHandler, error) {
	if len(secret) < 32 || service == nil || now == nil {
		return nil, errors.New("creation source handler requires a service, clock and secret of at least 32 bytes")
	}
	return &SourceHandler{service: service, secret: []byte(secret), now: now}, nil
}

func (h *SourceHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /internal/creation/runs/{run_id}/source", h.read)
}

func (h *SourceHandler) read(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		writeError(w, r, app.Problem("validation_failed", 422))
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	if err != nil {
		writeError(w, r, app.Problem("request_too_large", 413))
		return
	}
	tokens := r.Header.Values(sourceAuthorizationHeader)
	if len(tokens) != 1 || !h.authorized(tokens[0], r.Method, r.URL.EscapedPath(), body) {
		writeError(w, r, app.Problem("unauthenticated", 401))
		return
	}
	var input struct {
		PayloadHash string `json:"payload_hash"`
	}
	if decodeSourceObject(body, &input) != nil {
		writeError(w, r, app.Problem("validation_failed", 422))
		return
	}
	result, err := h.service.ReadSource(r.Context(), r.PathValue("run_id"), input.PayloadHash)
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	platformhttp.WriteJSON(w, http.StatusOK, result)
}

type sourceAuthorization struct {
	Audience  string `json:"audience"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	BodyHash  string `json:"body_hash"`
	ExpiresAt int64  `json:"expires_at"`
}

func (h *SourceHandler) authorized(token, method, path string, body []byte) bool {
	if len(token) > 4096 {
		return false
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return false
	}
	payload, err := base64.RawURLEncoding.Strict().DecodeString(parts[0])
	if err != nil {
		return false
	}
	signature, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, h.secret)
	_, _ = mac.Write([]byte(parts[0]))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return false
	}
	var claims sourceAuthorization
	if decodeSourceObject(payload, &claims) != nil {
		return false
	}
	now := h.now().Unix()
	hash := sha256.Sum256(body)
	return claims.Audience == "lanverse.creation.platform.source" && claims.Method == method && claims.Path == path && claims.BodyHash == hex.EncodeToString(hash[:]) && claims.ExpiresAt > now && claims.ExpiresAt <= now+60
}

// Both request objects are flat and closed. Reject duplicate fields before the
// typed decode so different parsers cannot authorize different representations.
func decodeSourceObject(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token != json.Delim('{') {
		return errors.New("expected JSON object")
	}
	seen := map[string]bool{}
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok || seen[key] {
			return errors.New("duplicate JSON field")
		}
		seen[key] = true
		var value json.RawMessage
		if err = decoder.Decode(&value); err != nil {
			return err
		}
	}
	if _, err = decoder.Token(); err != nil {
		return err
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return errors.New("trailing JSON data")
	}
	decoder = json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

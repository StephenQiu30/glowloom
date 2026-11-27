package provider

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
)

// VerifyProviderKey checks authentication on a provider's official read-only API.
func (c *Client) VerifyProviderKey(ctx context.Context, route domain.Route, key string) error {
	endpoint, ok := application.KeyEndpointFor(route)
	if !ok || key == "" || strings.ContainsAny(key, "\r\n\x00") {
		return domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	route.BaseURL = endpoint.BaseURL
	requestURL := endpoint.BaseURL + endpoint.Path
	if endpoint.Provider == "volcengine_speech" {
		return c.verifySpeechKey(ctx, route, key, requestURL)
	}
	method, body := http.MethodGet, ""
	switch endpoint.Provider {
	case "volcengine_motion", "volcengine_avatar":
		method = http.MethodPost
		// Zero is reserved for this read-only lookup, never a real task ID.
		// The provider must acknowledge the lookup with its normal receipt.
		model := "jimeng_dreamactor_m20_gen_video"
		if endpoint.Provider == "volcengine_avatar" {
			model = "jimeng_realman_avatar_picture_omni_v15"
		}
		if route.UpstreamModel != model {
			return domain.ErrProviderKeyCheckUnsupported
		}
		body = `{"req_key":"` + model + `","task_id":"0"}`
	}
	var payload io.Reader
	if body != "" {
		payload = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, payload)
	if err != nil {
		return domain.ErrUnavailable
	}
	resp, err := (fenced{client: c, route: route, key: key}).Do(req)
	if err != nil {
		return domain.ErrUnavailable
	}
	defer func() { _ = resp.Body.Close() }()
	if err := keyCheckStatus(resp.StatusCode, http.StatusOK); err != nil {
		return err
	}
	const maxResponse = 4 << 20
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil || len(raw) > maxResponse {
		return domain.ErrUnavailable
	}
	return readKeyResponse(endpoint.Provider, raw)
}

func keyCheckStatus(status, success int) error {
	switch status {
	case http.StatusUnauthorized:
		return domain.ErrProviderKeyRejected
	case http.StatusForbidden:
		return domain.ErrProviderKeyScope
	case success:
		return nil
	default:
		return domain.ErrUnavailable
	}
}

// verifySpeechKey performs only the authenticated HTTP Upgrade. No WebSocket
// frames, synthesis session or text are sent, even after a successful upgrade.
func (c *Client) verifySpeechKey(ctx context.Context, route domain.Route, key, requestURL string) error {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return domain.ErrUnavailable
	}
	websocketKey := base64.StdEncoding.EncodeToString(nonce[:])
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return domain.ErrUnavailable
	}
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Key", websocketKey)
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("X-Api-Resource-Id", "seed-tts-2.0")
	req.Header.Set("X-Api-Connect-Id", uuid.NewString())
	resp, err := (fenced{client: c, route: route, key: key, speechKeyHandshake: true}).Do(req)
	if err != nil {
		return domain.ErrUnavailable
	}
	defer func() { _ = resp.Body.Close() }()
	if err := keyCheckStatus(resp.StatusCode, http.StatusSwitchingProtocols); err != nil {
		return err
	}
	// RFC 6455 requires SHA-1 for this public protocol echo, not for authentication.
	accept := sha1.Sum([]byte(websocketKey + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	if !strings.EqualFold(resp.Header.Get("Upgrade"), "websocket") ||
		resp.Header.Get("Sec-WebSocket-Accept") != base64.StdEncoding.EncodeToString(accept[:]) {
		return domain.ErrUnavailable
	}
	for token := range strings.SplitSeq(resp.Header.Get("Connection"), ",") {
		if strings.EqualFold(strings.TrimSpace(token), "upgrade") {
			return nil
		}
	}
	return domain.ErrUnavailable
}

func readKeyResponse(provider string, raw []byte) error {
	switch provider {
	case "volcengine_motion", "volcengine_avatar":
		var lookup struct {
			Code      int    `json:"code"`
			RequestID string `json:"request_id"`
			Data      struct {
				Status string `json:"status"`
			} `json:"data"`
		}
		if json.Unmarshal(raw, &lookup) == nil && lookup.RequestID != "" {
			if lookup.Code == 50400 {
				return domain.ErrProviderKeyScope
			}
			if lookup.Code == 10000 && lookup.Data.Status == "not_found" {
				return nil
			}
		}
	case "openrouter":
		var key struct {
			Data struct {
				Label        string `json:"label"`
				Management   bool   `json:"is_management_key"`
				Provisioning bool   `json:"is_provisioning_key"`
			} `json:"data"`
		}
		if json.Unmarshal(raw, &key) == nil && key.Data.Label != "" {
			if key.Data.Management || key.Data.Provisioning {
				return domain.ErrProviderKeyScope
			}
			return nil
		}
	case "vercel":
		var credits struct {
			Balance *json.Number `json:"balance"`
			Used    *json.Number `json:"total_used"`
		}
		if json.Unmarshal(raw, &credits) == nil && credits.Balance != nil && credits.Used != nil {
			return nil
		}
	case "byteplus":
		var tasks struct {
			Items []json.RawMessage `json:"items"`
			Total *int              `json:"total"`
		}
		if json.Unmarshal(raw, &tasks) == nil && tasks.Items != nil && tasks.Total != nil {
			return nil
		}
	default:
		var list struct {
			Object string            `json:"object"`
			Data   []json.RawMessage `json:"data"`
		}
		if json.Unmarshal(raw, &list) == nil && list.Data != nil &&
			(list.Object == "list" || provider == "anthropic" && list.Object == "") {
			return nil
		}
	}
	return domain.ErrUnavailable
}

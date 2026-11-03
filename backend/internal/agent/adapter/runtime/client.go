// Package runtime implements the signed Go-to-Host control protocol.
package runtime

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	"github.com/google/uuid"
)

const protocol = "glowloom.agent-runtime/v1"

// Client sends a bounded, signed request to one configured Host origin.
type Client struct {
	origin *url.URL
	key    []byte
	http   *http.Client
	now    func() time.Time
}

func NewClient(origin, signingKey string) (*Client, error) {
	u, err := url.Parse(strings.TrimSpace(origin))
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || len(signingKey) < 32 {
		return nil, fmt.Errorf("invalid_agent_runtime_configuration")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	return &Client{
		origin: &url.URL{Scheme: u.Scheme, Host: u.Host},
		key:    []byte(signingKey),
		http: &http.Client{
			Transport: transport,
			Timeout:   12 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		now: time.Now,
	}, nil
}

type envelope struct {
	Protocol   string                  `json:"protocol"`
	Identity   domain.RuntimeIdentity `json:"identity"`
	Model      domain.DialogueModel    `json:"model"`
	Command    app.RuntimeInput        `json:"command"`
	PayloadHash string                 `json:"payload_hash"`
}

func (c *Client) Submit(ctx context.Context, delivery app.RuntimeDelivery) error {
	hash, err := domain.CanonicalHash(map[string]any{
		"identity": delivery.Identity,
		"model":    delivery.Model,
		"command":  delivery.Command,
	})
	if err != nil {
		return err
	}
	body, err := json.Marshal(envelope{Protocol: protocol, Identity: delivery.Identity, Model: delivery.Model, Command: delivery.Command, PayloadHash: hash})
	if err != nil {
		return err
	}
	var receipt struct {
		RequestID uuid.UUID `json:"request_id"`
	}
	if err := c.request(ctx, http.MethodPost, "/internal/v1/commands", body, http.StatusAccepted, &receipt); err != nil {
		return err
	}
	if receipt.RequestID != delivery.Command.RequestID {
		return fmt.Errorf("invalid_agent_runtime_receipt")
	}
	return nil
}

func (c *Client) Drain(ctx context.Context, identity domain.RuntimeIdentity) error {
	hash, err := domain.CanonicalHash(map[string]any{"identity": identity})
	if err != nil {
		return err
	}
	body, err := json.Marshal(struct {
		Protocol    string                  `json:"protocol"`
		Identity    domain.RuntimeIdentity `json:"identity"`
		PayloadHash string                  `json:"payload_hash"`
	}{protocol, identity, hash})
	if err != nil {
		return err
	}
	var receipt struct {
		Status string `json:"status"`
	}
	if err := c.request(ctx, http.MethodPost, "/internal/v1/owners/"+identity.RuntimeSessionID.String()+"/drain", body, http.StatusOK, &receipt); err != nil {
		return err
	}
	if receipt.Status != "drained" {
		return fmt.Errorf("invalid_agent_runtime_receipt")
	}
	return nil
}

func (c *Client) request(ctx context.Context, method, path string, body []byte, expected int, result any) error {
	expires := strconv.FormatInt(c.now().Unix()+120, 10)
	digest := sha256.Sum256(body)
	message := method + "\n" + path + "\n" + expires + "\n" + hex.EncodeToString(digest[:])
	mac := hmac.New(sha256.New, c.key)
	_, _ = mac.Write([]byte(message))
	req, err := http.NewRequestWithContext(ctx, method, c.origin.String()+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Glowloom-Runtime-Expires", expires)
	req.Header.Set("X-Glowloom-Runtime-Signature", hex.EncodeToString(mac.Sum(nil)))
	response, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("agent_runtime_unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != expected {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1024))
		return fmt.Errorf("agent_runtime_http_%d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return fmt.Errorf("invalid_agent_runtime_response")
	}
	if err := json.Unmarshal(data, result); err != nil {
		return fmt.Errorf("invalid_agent_runtime_response")
	}
	return nil
}

var _ app.RuntimeTransport = (*Client)(nil)

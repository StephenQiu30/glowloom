package agenthttp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	app "github.com/StephenQiu30/lanverse/backend/internal/production/creation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/creation/domain"
)

// Config relocates only an explicitly declared origin of the same Creation service.
// Historical run endpoints and engine identities remain unchanged.
type Config struct {
	Secret        string
	RelocatedFrom string
	Endpoint      string
}

type Client struct {
	relocatedFrom string
	endpoint      string
	secret        []byte
	http          *http.Client
	now           func() time.Time
}

func New(config Config, httpClient *http.Client, now func() time.Time) (*Client, error) {
	if len(config.Secret) < 32 {
		return nil, errors.New("creation agent secret must contain at least 32 bytes")
	}
	client := http.Client{Timeout: 10 * time.Second}
	if httpClient != nil {
		client = *httpClient
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if config.RelocatedFrom != "" && config.Endpoint == "" {
		return nil, errors.New("creation relocation target is required")
	}
	return &Client{secret: []byte(config.Secret), http: &client, now: now, relocatedFrom: config.RelocatedFrom, endpoint: config.Endpoint}, nil
}
func (c *Client) origin(run domain.Run) string {
	if c.relocatedFrom != "" && run.Endpoint == c.relocatedFrom {
		return strings.TrimRight(c.endpoint, "/")
	}
	return strings.TrimRight(run.Endpoint, "/")
}

func (c *Client) Lookup(ctx context.Context, run domain.Run) (domain.Acceptance, error) {
	return c.request(ctx, run, http.MethodGet, "/internal/creation/commands/"+run.Command.CommandID, nil)
}
func (c *Client) Accept(ctx context.Context, run domain.Run) (domain.Acceptance, error) {
	body, err := json.Marshal(run.Command)
	if err != nil {
		return domain.Acceptance{}, err
	}
	body, err = canonical.JSON(body)
	if err != nil {
		return domain.Acceptance{}, err
	}
	hash := sha256.Sum256(body)
	if hex.EncodeToString(hash[:]) != run.PayloadHash {
		return domain.Acceptance{}, errors.New("creation payload hash changed before dispatch")
	}
	return c.request(ctx, run, http.MethodPost, "/internal/creation/commands", body)
}

type authorization struct {
	Audience  string `json:"audience"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	BodyHash  string `json:"body_hash"`
	ExpiresAt int64  `json:"expires_at"`
}

func (c *Client) request(ctx context.Context, run domain.Run, method, path string, body []byte) (domain.Acceptance, error) {
	request, err := http.NewRequestWithContext(ctx, method, c.origin(run)+path, bytes.NewReader(body))
	if err != nil {
		return domain.Acceptance{}, err
	}
	hash := sha256.Sum256(body)
	payload, err := json.Marshal(authorization{Audience: "lanverse.creation.command", Method: method, Path: request.URL.EscapedPath(), BodyHash: hex.EncodeToString(hash[:]), ExpiresAt: c.now().Add(time.Minute).Unix()})
	if err != nil {
		return domain.Acceptance{}, err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	signature := hmac.New(sha256.New, c.secret)
	_, _ = signature.Write([]byte(encoded))
	request.Header.Set("X-Lanverse-Creation-Authorization", encoded+"."+base64.RawURLEncoding.EncodeToString(signature.Sum(nil)))
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return domain.Acceptance{}, fmt.Errorf("creation agent request: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == 404 && method == http.MethodGet {
		return domain.Acceptance{}, app.ErrNotFound
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusAccepted {
		return domain.Acceptance{}, app.Problem("agent_http_error", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 65537))
	if err != nil {
		return domain.Acceptance{}, err
	}
	if len(raw) > 65536 {
		return domain.Acceptance{}, errors.New("creation receipt exceeds limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var receipt domain.Acceptance
	if err = decoder.Decode(&receipt); err != nil {
		return domain.Acceptance{}, err
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return domain.Acceptance{}, errors.New("creation receipt has trailing data")
	}
	if err = app.ValidateAcceptance(run, receipt); err != nil {
		return domain.Acceptance{}, err
	}
	return receipt, nil
}

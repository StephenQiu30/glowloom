package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

const maxResultBytes = 16 << 20

type GrantIssuer interface {
	Issue(contract.Invocation) (string, error)
}

type HTTP struct {
	endpoint string
	client   *http.Client
	grants   GrantIssuer
}

func New(rawURL string, grants GrantIssuer, httpClient *http.Client) (*HTTP, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("AGENT_URL must be an absolute HTTP URL")
	}
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &HTTP{endpoint: strings.TrimRight(parsed.String(), "/") + "/internal/v1/invocations", client: httpClient, grants: grants}, nil
}

func (client *HTTP) Invoke(ctx context.Context, invocation contract.Invocation) (contract.Result, error) {
	if err := invocation.Validate(); err != nil {
		return contract.Result{}, err
	}
	body, err := json.Marshal(invocation)
	if err != nil {
		return contract.Result{}, err
	}
	grantValue, err := client.grants.Issue(invocation)
	if err != nil {
		return contract.Result{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.endpoint, bytes.NewReader(body))
	if err != nil {
		return contract.Result{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Lanverse-Execution-Grant", grantValue)
	response, err := client.client.Do(request)
	if err != nil {
		return contract.Result{}, fmt.Errorf("agent invocation outcome unknown: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return contract.Result{}, fmt.Errorf("agent invocation returned HTTP %d", response.StatusCode)
	}
	var result contract.Result
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxResultBytes+1))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&result); err != nil {
		return contract.Result{}, fmt.Errorf("decode agent result: %w", err)
	}
	if err = result.ValidateFor(invocation); err != nil {
		return contract.Result{}, err
	}
	return result, nil
}

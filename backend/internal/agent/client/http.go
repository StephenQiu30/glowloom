package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

const maxResultBytes = 16 << 20

type GrantIssuer interface {
	Issue(contract.StageInvocation, int, int64) (string, error)
}

type HTTP struct {
	runtimes contract.RuntimeCatalog
	client   *http.Client
	grants   GrantIssuer
}

func New(runtimes contract.RuntimeCatalog, grants GrantIssuer, httpClient *http.Client) *HTTP {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &HTTP{runtimes: runtimes, client: httpClient, grants: grants}
}

func (client *HTTP) Invoke(ctx context.Context, invocation contract.StageInvocation, attempt int, fencingToken int64) (contract.StageResult, error) {
	if err := invocation.Validate(); err != nil {
		return contract.StageResult{}, err
	}
	runtime, err := client.runtimes.Resolve(invocation.ExecutionPolicy.SkillBundleHash)
	if err != nil {
		return contract.StageResult{}, err
	}
	body, err := json.Marshal(invocation)
	if err != nil {
		return contract.StageResult{}, err
	}
	grantValue, err := client.grants.Issue(invocation, attempt, fencingToken)
	if err != nil {
		return contract.StageResult{}, err
	}
	endpoint := strings.TrimRight(runtime.BaseURL, "/") + "/internal/v1/invocations"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return contract.StageResult{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Lanverse-Execution-Grant", grantValue)
	response, err := client.client.Do(request)
	if err != nil {
		return contract.StageResult{}, fmt.Errorf("agent invocation outcome unknown: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return contract.StageResult{}, fmt.Errorf("agent invocation returned HTTP %d", response.StatusCode)
	}
	encoded, err := io.ReadAll(io.LimitReader(response.Body, maxResultBytes+1))
	if err != nil || len(encoded) > maxResultBytes {
		return contract.StageResult{}, errorsOrLimit(err)
	}
	result, err := contract.DecodeStageResult(encoded)
	if err != nil {
		return contract.StageResult{}, fmt.Errorf("decode agent result: %w", err)
	}
	if err = result.ValidateFor(invocation); err != nil {
		return contract.StageResult{}, err
	}
	return result, nil
}

func errorsOrLimit(err error) error {
	if err != nil {
		return fmt.Errorf("read agent result: %w", err)
	}
	return fmt.Errorf("agent result exceeds %d bytes", maxResultBytes)
}

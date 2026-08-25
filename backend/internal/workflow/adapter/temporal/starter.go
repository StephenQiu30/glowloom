package temporal

import (
	"context"
	"errors"
	"fmt"
	"strings"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	temporalclient "go.temporal.io/sdk/client"
	"go.temporal.io/sdk/converter"

	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

const inputHashMemoKey = "lanverse_input_hash"

type Config struct {
	Address, Namespace, TaskQueue string
}

type Client struct {
	client        temporalclient.Client
	namespace     string
	taskQueue     string
	dataConverter converter.DataConverter
}

func New(config Config) (*Client, error) {
	config.Address = strings.TrimSpace(config.Address)
	config.Namespace = strings.TrimSpace(config.Namespace)
	config.TaskQueue = strings.TrimSpace(config.TaskQueue)
	if config.Address == "" || config.Namespace == "" || config.TaskQueue == "" || len(config.TaskQueue) > 255 {
		return nil, errors.New("invalid Temporal client configuration")
	}
	temporalClient, err := temporalclient.Dial(temporalclient.Options{HostPort: config.Address, Namespace: config.Namespace})
	if err != nil {
		return nil, fmt.Errorf("connect Temporal service: %w", err)
	}
	return &Client{
		client: temporalClient, namespace: config.Namespace, taskQueue: config.TaskQueue,
		dataConverter: converter.GetDefaultDataConverter(),
	}, nil
}

func (runtime *Client) Close() {
	if runtime != nil && runtime.client != nil {
		runtime.client.Close()
	}
}

func (runtime *Client) Start(ctx context.Context, request domain.StartRequest) (domain.StartObservation, error) {
	if runtime == nil || runtime.client == nil || !validRequest(request) {
		return domain.StartObservation{}, errors.New("invalid Temporal workflow start request")
	}
	_, err := runtime.client.ExecuteWorkflow(ctx, temporalclient.StartWorkflowOptions{
		ID: request.WorkflowID, TaskQueue: runtime.taskQueue,
		WorkflowIDConflictPolicy:                 enumspb.WORKFLOW_ID_CONFLICT_POLICY_FAIL,
		WorkflowIDReusePolicy:                    enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE,
		WorkflowExecutionErrorWhenAlreadyStarted: true,
		Memo:                                     map[string]interface{}{inputHashMemoKey: request.InputHash},
	}, request.WorkflowType, request)
	if err == nil {
		return domain.StartObservation{Outcome: domain.StartOutcomeStarted, ObservedInputHash: request.InputHash}, nil
	}
	var alreadyStarted *serviceerror.WorkflowExecutionAlreadyStarted
	if errors.As(err, &alreadyStarted) {
		return runtime.describeInputHash(ctx, request.WorkflowID)
	}
	observation, describeErr := runtime.describeInputHash(ctx, request.WorkflowID)
	if describeErr == nil {
		return observation, nil
	}
	var notFound *serviceerror.NotFound
	if errors.As(describeErr, &notFound) {
		return domain.StartObservation{Outcome: domain.StartOutcomeUnknown}, err
	}
	return domain.StartObservation{Outcome: domain.StartOutcomeUnknown}, errors.Join(err, describeErr)
}

func (runtime *Client) describeInputHash(ctx context.Context, workflowID string) (domain.StartObservation, error) {
	description, err := runtime.client.DescribeWorkflowExecution(ctx, workflowID, "")
	if err != nil {
		return domain.StartObservation{}, err
	}
	if description == nil || description.WorkflowExecutionInfo == nil || description.WorkflowExecutionInfo.Memo == nil {
		return domain.StartObservation{}, errors.New("Temporal workflow description has no input hash memo")
	}
	payload, exists := description.WorkflowExecutionInfo.Memo.Fields[inputHashMemoKey]
	if !exists {
		return domain.StartObservation{}, errors.New("Temporal workflow input hash memo is missing")
	}
	var inputHash string
	if err = runtime.dataConverter.FromPayload(payload, &inputHash); err != nil {
		return domain.StartObservation{}, fmt.Errorf("decode Temporal workflow input hash memo: %w", err)
	}
	if len(inputHash) != 64 {
		return domain.StartObservation{}, errors.New("Temporal workflow input hash memo is invalid")
	}
	return domain.StartObservation{Outcome: domain.StartOutcomeAlreadyStarted, ObservedInputHash: inputHash}, nil
}

func validRequest(request domain.StartRequest) bool {
	return request.WorkflowID != "" && request.WorkflowType != "" && request.WorkflowRunID != "" &&
		request.DefinitionVersionID != "" && request.RunInputSnapshotID != "" &&
		len(request.DefinitionContentHash) == 64 && len(request.InputSnapshotHash) == 64 && len(request.InputHash) == 64
}

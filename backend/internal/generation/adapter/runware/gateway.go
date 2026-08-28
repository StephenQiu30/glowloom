package runware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
)

const (
	runwareEndpoint       = "https://api.runware.ai/v1"
	runwareProviderKey    = "runware"
	runwareModelKey       = "runware:z-image@turbo"
	runwareCredentialRef  = "env/runware_api_key"
	runwareOutputHost     = "im.runware.ai"
	maximumResponseBytes  = 1 << 20
	maximumRequestTimeout = 2 * time.Minute
)

var runwareFailureCodePattern = regexp.MustCompile(`[^a-z0-9._-]+`)

type ImageStager interface {
	StageImage(context.Context, StageImageRequest) (application.ProviderOutput, error)
}

type StageImageRequest struct {
	WorkspaceID, ProjectID, ProviderJobID string
	ImageUUID, ImageURL                   string
	Width, Height                         int
	OutputFormat                          string
}

type Config struct {
	APIKey         string
	Client         *http.Client
	Stager         ImageStager
	RequestTimeout time.Duration
}

type Gateway struct {
	apiKey         string
	client         *http.Client
	stager         ImageStager
	requestTimeout time.Duration
}

type imageInferenceTask struct {
	TaskType       string `json:"taskType"`
	TaskUUID       string `json:"taskUUID"`
	Model          string `json:"model"`
	PositivePrompt string `json:"positivePrompt"`
	NegativePrompt string `json:"negativePrompt"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	NumberResults  int    `json:"numberResults"`
	OutputFormat   string `json:"outputFormat"`
	OutputType     string `json:"outputType"`
	DeliveryMethod string `json:"deliveryMethod"`
}

type lookupTask struct {
	TaskType string `json:"taskType"`
	TaskUUID string `json:"taskUUID"`
}

type responseEnvelope struct {
	Data   []responseItem  `json:"data"`
	Errors []responseError `json:"errors"`
}

type responseItem struct {
	TaskType  string          `json:"taskType"`
	TaskUUID  string          `json:"taskUUID"`
	Status    string          `json:"status"`
	ImageUUID string          `json:"imageUUID"`
	ImageURL  string          `json:"imageURL"`
	Request   json.RawMessage `json:"request"`
	Response  json.RawMessage `json:"response"`
}

type responseError struct {
	Code     string `json:"code"`
	TaskUUID string `json:"taskUUID"`
}

func New(config Config) (*Gateway, error) {
	config.APIKey = strings.TrimSpace(config.APIKey)
	if config.APIKey == "" || config.Client == nil || config.Stager == nil || config.RequestTimeout <= 0 ||
		config.RequestTimeout > maximumRequestTimeout {
		return nil, errors.New("invalid Runware gateway configuration")
	}
	return &Gateway{
		apiKey: config.APIKey, client: config.Client, stager: config.Stager, requestTimeout: config.RequestTimeout,
	}, nil
}

func (gateway *Gateway) Submit(
	ctx context.Context,
	submission application.ProviderSubmission,
) (application.ProviderOutcome, error) {
	payload, err := gateway.referenceAssetPayload(submission)
	if err != nil {
		return application.ProviderOutcome{}, err
	}
	envelope, err := gateway.execute(ctx, imageInferenceTask{
		TaskType: "imageInference", TaskUUID: submission.ProviderJobID, Model: submission.ModelKey,
		PositivePrompt: payload.PositivePrompt, NegativePrompt: payload.NegativePrompt,
		Width: payload.Width, Height: payload.Height, NumberResults: payload.NumberResults,
		OutputFormat: payload.OutputFormat, OutputType: "URL", DeliveryMethod: "async",
	})
	if err != nil {
		return application.ProviderOutcome{}, err
	}
	if len(envelope.Errors) > 0 {
		if taskNotFound(envelope.Errors, submission.ProviderJobID) {
			return application.ProviderOutcome{
				Status: application.ProviderOutcomeUnknown, ProviderJobKey: submission.ProviderJobID,
			}, nil
		}
		return failedOutcome(submission.ProviderJobID, envelope.Errors)
	}
	items, completed, err := classifyImageItems(envelope.Data, submission.ProviderJobID, payload.NumberResults)
	if err != nil {
		return application.ProviderOutcome{}, err
	}
	if completed {
		return gateway.stageCompletedImages(ctx, submission, payload, items)
	}
	return application.ProviderOutcome{
		Status: application.ProviderOutcomeAccepted, ProviderJobKey: submission.ProviderJobID,
	}, nil
}

func (gateway *Gateway) Query(
	ctx context.Context,
	submission application.ProviderSubmission,
) (application.ProviderOutcome, error) {
	payload, err := gateway.referenceAssetPayload(submission)
	if err != nil {
		return application.ProviderOutcome{}, err
	}
	envelope, err := gateway.execute(ctx, lookupTask{TaskType: "getResponse", TaskUUID: submission.ProviderJobID})
	if err != nil {
		return application.ProviderOutcome{}, err
	}
	if len(envelope.Errors) > 0 {
		if taskNotFound(envelope.Errors, submission.ProviderJobID) {
			return application.ProviderOutcome{
				Status: application.ProviderOutcomeUnknown, ProviderJobKey: submission.ProviderJobID,
			}, nil
		}
		return failedOutcome(submission.ProviderJobID, envelope.Errors)
	}
	if len(envelope.Data) == 0 {
		envelope, err = gateway.taskDetails(ctx, submission, payload)
		if err != nil {
			return application.ProviderOutcome{}, err
		}
		if len(envelope.Errors) > 0 {
			if taskNotFound(envelope.Errors, submission.ProviderJobID) {
				return application.ProviderOutcome{
					Status: application.ProviderOutcomeUnknown, ProviderJobKey: submission.ProviderJobID,
				}, nil
			}
			return failedOutcome(submission.ProviderJobID, envelope.Errors)
		}
		if len(envelope.Data) == 0 {
			return application.ProviderOutcome{
				Status: application.ProviderOutcomeUnknown, ProviderJobKey: submission.ProviderJobID,
			}, nil
		}
	}
	items, completed, err := classifyImageItems(envelope.Data, submission.ProviderJobID, payload.NumberResults)
	if err != nil {
		return application.ProviderOutcome{}, err
	}
	if !completed {
		return application.ProviderOutcome{
			Status: application.ProviderOutcomeAccepted, ProviderJobKey: submission.ProviderJobID,
		}, nil
	}
	return gateway.stageCompletedImages(ctx, submission, payload, items)
}

func (gateway *Gateway) stageCompletedImages(
	ctx context.Context,
	submission application.ProviderSubmission,
	payload domain.ReferenceAssetTarget,
	items []responseItem,
) (application.ProviderOutcome, error) {
	outputs := make([]application.ProviderOutput, len(items))
	for index, item := range items {
		output, stageErr := gateway.stager.StageImage(ctx, StageImageRequest{
			WorkspaceID: submission.WorkspaceID, ProjectID: submission.ProjectID,
			ProviderJobID: submission.ProviderJobID, ImageUUID: item.ImageUUID, ImageURL: item.ImageURL,
			Width: payload.Width, Height: payload.Height, OutputFormat: payload.OutputFormat,
		})
		if stageErr != nil {
			return application.ProviderOutcome{}, errors.New("Runware output staging failed")
		}
		if !validStagedOutput(output, submission, item, payload) {
			return application.ProviderOutcome{}, errors.New("Runware staged output identity drifted")
		}
		outputs[index] = output
	}
	eventID, err := providerEventID(submission.ProviderJobID, items)
	if err != nil {
		return application.ProviderOutcome{}, err
	}
	return application.ProviderOutcome{
		Status: application.ProviderOutcomeSucceeded, ProviderJobKey: submission.ProviderJobID,
		ProviderEventID: eventID, ActualUnits: int64(len(outputs)), Outputs: outputs,
	}, nil
}

func (gateway *Gateway) taskDetails(
	ctx context.Context,
	submission application.ProviderSubmission,
	payload domain.ReferenceAssetTarget,
) (responseEnvelope, error) {
	history, err := gateway.execute(ctx, lookupTask{TaskType: "getTaskDetails", TaskUUID: submission.ProviderJobID})
	if err != nil || len(history.Errors) > 0 {
		return history, err
	}
	if len(history.Data) != 1 || history.Data[0].TaskType != "getTaskDetails" ||
		history.Data[0].TaskUUID != submission.ProviderJobID || len(history.Data[0].Request) == 0 ||
		len(history.Data[0].Response) == 0 {
		return responseEnvelope{}, errors.New("Runware task details identity drifted")
	}
	var original []imageInferenceTask
	if json.Unmarshal(history.Data[0].Request, &original) != nil || len(original) != 1 ||
		original[0] != (imageInferenceTask{
			TaskType: "imageInference", TaskUUID: submission.ProviderJobID, Model: submission.ModelKey,
			PositivePrompt: payload.PositivePrompt, NegativePrompt: payload.NegativePrompt,
			Width: payload.Width, Height: payload.Height, NumberResults: payload.NumberResults,
			OutputFormat: payload.OutputFormat, OutputType: "URL", DeliveryMethod: "async",
		}) {
		return responseEnvelope{}, errors.New("Runware original request does not match the frozen GenerationTarget")
	}
	var originalResponse responseEnvelope
	if json.Unmarshal(history.Data[0].Response, &originalResponse) != nil {
		return responseEnvelope{}, errors.New("invalid Runware task details response")
	}
	return originalResponse, nil
}

func (gateway *Gateway) referenceAssetPayload(
	submission application.ProviderSubmission,
) (domain.ReferenceAssetTarget, error) {
	if gateway == nil || gateway.client == nil || gateway.stager == nil ||
		submission.ProviderKey != runwareProviderKey || submission.ModelKey != runwareModelKey ||
		submission.CredentialRef != runwareCredentialRef || submission.WorkspaceID != submission.Target.WorkspaceID ||
		submission.ProjectID != submission.Target.ProjectID || submission.InputHash != submission.Target.TargetHash ||
		submission.Units != 4 || submission.Target.Kind != domain.GenerationTargetReferenceAsset ||
		submission.Target.ReferenceAsset == nil || submission.Target.ShotFrame != nil {
		return domain.ReferenceAssetTarget{}, errors.New("invalid Runware reference_asset submission")
	}
	for _, identifier := range []string{
		submission.WorkspaceID, submission.ProjectID, submission.ProviderJobID, submission.RequestID, submission.IntentID,
	} {
		if _, err := uuid.Parse(strings.TrimSpace(identifier)); err != nil {
			return domain.ReferenceAssetTarget{}, errors.New("invalid Runware submission identity")
		}
	}
	if strings.TrimSpace(submission.RequestKey) == "" || domain.ValidateGenerationTarget(submission.Target) != nil {
		return domain.ReferenceAssetTarget{}, errors.New("invalid Runware GenerationTarget snapshot")
	}
	return *submission.Target.ReferenceAsset, nil
}

func (gateway *Gateway) execute(ctx context.Context, task any) (responseEnvelope, error) {
	encoded, err := json.Marshal([]any{task})
	if err != nil {
		return responseEnvelope{}, errors.New("encode Runware request")
	}
	requestContext, cancel := context.WithTimeout(ctx, gateway.requestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodPost, runwareEndpoint, bytes.NewReader(encoded))
	if err != nil {
		return responseEnvelope{}, errors.New("build Runware request")
	}
	request.Header.Set("Authorization", "Bearer "+gateway.apiKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := gateway.client.Do(request)
	if err != nil {
		return responseEnvelope{}, errors.New("Runware request outcome is unknown")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return responseEnvelope{}, fmt.Errorf("Runware returned HTTP status %d", response.StatusCode)
	}
	limited := io.LimitReader(response.Body, maximumResponseBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil || len(body) == 0 || len(body) > maximumResponseBytes {
		return responseEnvelope{}, errors.New("invalid Runware response body")
	}
	var envelope responseEnvelope
	if err = json.Unmarshal(body, &envelope); err != nil {
		return responseEnvelope{}, errors.New("invalid Runware response envelope")
	}
	return envelope, nil
}

func failedOutcome(taskUUID string, values []responseError) (application.ProviderOutcome, error) {
	if len(values) == 0 {
		return application.ProviderOutcome{}, errors.New("Runware error response identity drifted")
	}
	codes := make([]string, 0, len(values))
	for _, value := range values {
		if value.TaskUUID != "" && value.TaskUUID != taskUUID {
			return application.ProviderOutcome{}, errors.New("Runware error response identity drifted")
		}
		code := strings.ToLower(strings.TrimSpace(value.Code))
		code = strings.Trim(runwareFailureCodePattern.ReplaceAllString(code, "_"), "_.-")
		if code == "" {
			code = "provider_error"
		}
		if len(code) > 100 {
			code = code[:100]
		}
		codes = append(codes, code)
	}
	slices.Sort(codes)
	codes = slices.Compact(codes)
	eventDigest := sha256.Sum256([]byte(taskUUID + "\x00" + strings.Join(codes, "\x00")))
	return application.ProviderOutcome{
		Status: application.ProviderOutcomeFailed, ProviderJobKey: taskUUID,
		ProviderEventID: hex.EncodeToString(eventDigest[:]), FailureCode: "runware_" + codes[0],
	}, nil
}

func taskNotFound(values []responseError, taskUUID string) bool {
	return len(values) == 1 && (values[0].TaskUUID == "" || values[0].TaskUUID == taskUUID) &&
		strings.EqualFold(strings.TrimSpace(values[0].Code), "taskNotFound")
}

func classifyImageItems(values []responseItem, taskUUID string, expected int) ([]responseItem, bool, error) {
	if len(values) == 0 || len(values) > expected {
		return nil, false, errors.New("Runware result count drifted")
	}
	items := append([]responseItem(nil), values...)
	completed := true
	seenImages := make(map[string]struct{}, len(items))
	for _, item := range items {
		status := strings.TrimSpace(item.Status)
		if item.TaskType != "imageInference" || item.TaskUUID != taskUUID ||
			!oneOfRunwareStatus(status, "", "processing", "success") {
			return nil, false, errors.New("Runware image result identity drifted")
		}
		if status == "processing" {
			if item.ImageUUID != "" || item.ImageURL != "" {
				return nil, false, errors.New("Runware processing result contains an output")
			}
			completed = false
			continue
		}
		if item.ImageUUID == "" && item.ImageURL == "" {
			if status == "success" {
				return nil, false, errors.New("Runware successful result is missing its output")
			}
			completed = false
			continue
		}
		parsedURL, urlErr := url.Parse(item.ImageURL)
		_, imageErr := uuid.Parse(item.ImageUUID)
		if imageErr != nil || urlErr != nil ||
			parsedURL.Scheme != "https" || parsedURL.Hostname() != runwareOutputHost || parsedURL.User != nil ||
			parsedURL.RawQuery != "" || parsedURL.Fragment != "" {
			return nil, false, errors.New("Runware image result identity drifted")
		}
		if _, duplicate := seenImages[item.ImageUUID]; duplicate {
			return nil, false, errors.New("Runware image result identity drifted")
		}
		seenImages[item.ImageUUID] = struct{}{}
	}
	if len(seenImages) != expected {
		completed = false
	}
	slices.SortFunc(items, func(left, right responseItem) int { return strings.Compare(left.ImageUUID, right.ImageUUID) })
	return items, completed, nil
}

func validStagedOutput(
	output application.ProviderOutput,
	submission application.ProviderSubmission,
	item responseItem,
	payload domain.ReferenceAssetTarget,
) bool {
	prefix := "staging/" + submission.WorkspaceID + "/" + submission.ProviderJobID + "/"
	decodedSHA, shaErr := hex.DecodeString(output.SHA256)
	return output.OutputKey == item.ImageUUID && strings.HasPrefix(output.StagingObjectKey, prefix) &&
		!strings.Contains(output.StagingObjectKey, "..") && shaErr == nil && len(decodedSHA) == sha256.Size && output.Bytes > 0 &&
		output.MediaType == "image/png" && output.Width == payload.Width && output.Height == payload.Height
}

func providerEventID(taskUUID string, items []responseItem) (string, error) {
	type eventImage struct {
		ImageUUID string `json:"image_uuid"`
		ImageURL  string `json:"image_url"`
	}
	images := make([]eventImage, len(items))
	for index, item := range items {
		images[index] = eventImage{ImageUUID: item.ImageUUID, ImageURL: item.ImageURL}
	}
	encoded, err := json.Marshal(struct {
		TaskUUID string       `json:"task_uuid"`
		Images   []eventImage `json:"images"`
	}{TaskUUID: taskUUID, Images: images})
	if err != nil {
		return "", errors.New("hash Runware response")
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func oneOfRunwareStatus(value string, allowed ...string) bool {
	return slices.Contains(allowed, strings.TrimSpace(value))
}

var _ application.ProviderGateway = (*Gateway)(nil)

package operation_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/codex"
	operationflow "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/workflow"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

func TestTrialRequestHasNoBillingOrArbitraryRuntimeFields(t *testing.T) {
	r := application.TrialRequest{RequestID: uuid.New(), Source: "codex", Kind: "chat", Model: "fixture-model", Prompt: "hello"}
	require.NoError(t, r.Validate())
	r.Source = "http://127.0.0.1"
	require.Error(t, r.Validate())
	r.Source = "codex"
	r.Kind = "video"
	require.Error(t, r.Validate())
	r.Kind = "chat"
	r.Prompt = ""
	require.Error(t, r.Validate())
	r.Kind = "probe"
	r.Model = ""
	require.NoError(t, r.Validate())
	r.Source = "cloud"
	require.Error(t, r.Validate())
}

func TestTrialWorkflowDoesNotRetryAnUncertainDispatch(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	calls := 0
	env.RegisterActivityWithOptions(func(context.Context, application.TrialInput) (application.TrialProgress, error) {
		calls++
		return application.TrialProgress{}, errors.New("response lost")
	}, activity.RegisterOptions{Name: "trial.Run"})
	env.ExecuteWorkflow(operationflow.TrialWorkflow, application.TrialInput{Request: application.TrialRequest{RequestID: uuid.New(), Source: "codex", Kind: "image", Model: "fixture", Prompt: "test"}})
	require.NoError(t, env.GetWorkflowError())
	var state application.TrialState
	require.NoError(t, env.GetWorkflowResult(&state))
	require.Equal(t, "unknown", state.State)
	require.Equal(t, 1, calls)
}
func TestLocalInstalledCodexTrialProbe(t *testing.T) {
	binary := os.Getenv("LV_TEST_CODEX_BINARY")
	if binary == "" {
		t.Skip("explicit native CLI fixture required")
	}
	root := t.TempDir()
	require.NoError(t, os.Chmod(root, 0700))
	launcher, err := codex.NewLocalProcessLauncher(binary, root)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	status, err := codex.Probe(ctx, launcher, root)
	require.NoError(t, err)
	t.Logf("local CLI authenticated=%t image_generation=%t model_count=%d (no inference)", status.Authenticated, status.ImageGeneration, len(status.Models))
}

func TestLocalInstalledCodexTrialInference(t *testing.T) {
	mode := os.Getenv("LV_TEST_CODEX_INFERENCE")
	binary := os.Getenv("LV_TEST_CODEX_BINARY")
	if mode == "" || binary == "" {
		t.Skip("explicit local inference opt-in required")
	}
	root := t.TempDir()
	require.NoError(t, os.Chmod(root, 0700))
	launcher, err := codex.NewLocalProcessLauncher(binary, root)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Minute)
	defer cancel()
	status, err := codex.Probe(ctx, launcher, root)
	require.NoError(t, err)
	require.True(t, status.Authenticated)
	require.NotEmpty(t, status.Models)
	model := status.Models[0]
	if mode == "image" {
		require.True(t, status.ImageGeneration)
		image, outcome, err := codex.TrialImage(ctx, launcher, root, model, "Generate exactly one simple blue circle on a white background, no text. Save one PNG image in the task directory.")
		require.NoError(t, err)
		require.Equal(t, "completed", outcome)
		require.NotEmpty(t, image.Bytes)
		if dir := os.Getenv("LV_TEST_CODEX_ARTIFACT_DIR"); dir != "" {
			require.NoError(t, os.MkdirAll(dir, 0700))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "native-codex-image.png"), image.Bytes, 0600))
		}
		t.Logf("real image verified: model=%s mime=%s bytes=%d", model, image.MediaType, len(image.Bytes))
		return
	}
	session, err := codex.OpenChat(ctx, launcher, root, model)
	require.NoError(t, err)
	result, err := session.Turn(ctx, "Reply exactly with the token GLOWLOOM_POC_OK.", nil)
	require.NoError(t, err)
	require.Equal(t, "completed", result.Outcome)
	require.Contains(t, result.Text, "GLOWLOOM_POC_OK")
	thread := session.ThreadID()
	require.NoError(t, session.Close())
	resumed, err := codex.ResumeChat(ctx, launcher, root, model, thread)
	require.NoError(t, err)
	defer func() { _ = resumed.Close() }()
	result, err = resumed.Turn(ctx, "What exact token did you reply with in your previous answer? Reply only with that token.", nil)
	require.NoError(t, err)
	require.Equal(t, "completed", result.Outcome)
	require.Contains(t, result.Text, "GLOWLOOM_POC_OK")
	t.Logf("real two-turn chat and process resume verified: model=%s", model)
}

func TestTrialWorkflowContinuesOnlyItsOriginalPrivateChatAndDeduplicatesSignals(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	id, nextID := uuid.New(), uuid.New()
	var inputs []application.TrialInput
	env.RegisterActivityWithOptions(func(_ context.Context, in application.TrialInput) (application.TrialProgress, error) {
		inputs = append(inputs, in)
		return application.TrialProgress{State: "succeeded", Text: "answer", ThreadID: "private-owned-thread"}, nil
	}, activity.RegisterOptions{Name: "trial.Run"})
	env.RegisterDelayedCallback(func() { env.SignalWorkflow("turn", application.TrialMessage{RequestID: id, Prompt: "same"}) }, time.Second)
	env.RegisterDelayedCallback(func() { env.SignalWorkflow("turn", application.TrialMessage{RequestID: nextID, Prompt: "continue"}) }, 2*time.Second)
	env.RegisterDelayedCallback(func() { env.SignalWorkflow("cancel", true) }, 3*time.Second)
	env.ExecuteWorkflow(operationflow.TrialWorkflow, application.TrialInput{Request: application.TrialRequest{RequestID: id, Source: "codex", Kind: "chat", Model: "frozen-model", Prompt: "same"}})
	require.NoError(t, env.GetWorkflowError())
	require.Len(t, inputs, 2)
	require.Equal(t, "private-owned-thread", inputs[1].ThreadID)
	require.Equal(t, "frozen-model", inputs[1].Request.Model)
	require.Equal(t, nextID, inputs[1].Request.RequestID)
}
func TestTrialWorkflowVideoPollsFrozenTaskWithoutResubmitting(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	configID, keyID := uuid.New(), uuid.New()
	var actions []string
	env.RegisterActivityWithOptions(func(_ context.Context, in application.TrialInput) (application.TrialProgress, error) {
		actions = append(actions, in.Action)
		if len(actions) == 1 {
			return application.TrialProgress{State: "pending", TaskID: "known-task", ConfigVersionID: configID, CredentialID: keyID}, nil
		}
		require.Equal(t, "known-task", in.TaskID)
		require.Equal(t, configID, in.ConfigVersionID)
		require.Equal(t, keyID, in.CredentialID)
		return application.TrialProgress{State: "succeeded"}, nil
	}, activity.RegisterOptions{Name: "trial.Run"})
	env.ExecuteWorkflow(operationflow.TrialWorkflow, application.TrialInput{Request: application.TrialRequest{RequestID: uuid.New(), Source: "cloud", ProviderID: uuid.New(), Kind: "video", Model: "fixture", Prompt: "test"}})
	require.NoError(t, env.GetWorkflowError())
	require.Equal(t, []string{"submit", "query"}, actions)
}

func TestTrialWorkflowRetainsOriginalVideoIdentityWhenQueryActivityCrashes(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	configID, keyID := uuid.New(), uuid.New()
	env.RegisterActivityWithOptions(func(_ context.Context, in application.TrialInput) (application.TrialProgress, error) {
		if in.Action == "submit" {
			return application.TrialProgress{State: "pending", TaskID: "known-task", ConfigVersionID: configID, CredentialID: keyID}, nil
		}
		return application.TrialProgress{}, errors.New("query worker crashed")
	}, activity.RegisterOptions{Name: "trial.Run"})
	env.ExecuteWorkflow(operationflow.TrialWorkflow, application.TrialInput{Request: application.TrialRequest{RequestID: uuid.New(), Source: "cloud", ProviderID: uuid.New(), Kind: "video", Model: "fixture", Prompt: "test"}})
	var state application.TrialState
	require.NoError(t, env.GetWorkflowResult(&state))
	require.Equal(t, "unknown", state.State)
	require.Equal(t, "known-task", state.Turns[0].TaskID)
	require.Equal(t, configID, state.Turns[0].ConfigVersionID)
	require.Equal(t, keyID, state.Turns[0].CredentialID)
}

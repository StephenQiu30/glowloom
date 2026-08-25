package workflow_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/client"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/grant"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	biblegorm "github.com/StephenQiu30/lanverse/backend/internal/production/bible/adapter/gormdb"
	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
)

const (
	workerProcessHelperEnv = "LANVERSE_WORKFLOW_PROCESS_HELPER"
	workerProcessAgentEnv  = "LANVERSE_WORKFLOW_PROCESS_AGENT_URL"
	workerProcessSecretEnv = "LANVERSE_WORKFLOW_PROCESS_AGENT_SECRET"
	workerProcessSecret    = "lanverse-workflow-process-test-secret-32-bytes"
)

func TestBibleWorkerReclaimsInvocationAfterProcessRestart(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the PostgreSQL workflow journey")
	}

	ctx := context.Background()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatalf("synchronize GORM catalog: %v", err)
	}

	fixture := seedFailedBible(t, func(value any) error {
		return database.Create(value).Error
	})
	store := biblegorm.New(database)
	service := bibleapp.NewService(store, bibleapp.Config{
		Now:   func() time.Time { return time.Now().UTC() },
		NewID: uuid.NewString,
	})
	actor := bibleapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	if _, err = service.Resume(ctx, actor, bibleapp.ResumeCommand{
		BibleID: fixture.bibleID.String(), ExpectedRevision: fixture.bibleRevision,
		IdempotencyKey: "process-restart-resume-1",
	}); err != nil {
		t.Fatalf("queue bible before process restart: %v", err)
	}
	loadInvocation := func() model.AgentInvocation {
		var record model.AgentInvocation
		if loadErr := database.First(&record, "id = ?", fixture.invocationID).Error; loadErr != nil {
			t.Fatalf("reload workflow invocation: %v", loadErr)
		}
		return record
	}

	requests := make(chan contract.Invocation, 2)
	serverErrors := make(chan error, 2)
	var requestCount atomic.Int32
	grantVerifier, err := grant.NewSigner(workerProcessSecret, func() time.Time { return time.Now().UTC() })
	if err != nil {
		t.Fatalf("create grant verifier: %v", err)
	}
	agentServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		var invocation contract.Invocation
		if decodeErr := json.NewDecoder(request.Body).Decode(&invocation); decodeErr != nil {
			serverErrors <- decodeErr
			http.Error(response, "invalid invocation", http.StatusBadRequest)
			return
		}
		if verifyErr := grantVerifier.Verify(request.Header.Get("X-Lanverse-Execution-Grant"), invocation); verifyErr != nil {
			serverErrors <- verifyErr
			http.Error(response, "invalid grant", http.StatusUnauthorized)
			return
		}
		call := requestCount.Add(1)
		requests <- invocation
		if call == 1 {
			<-request.Context().Done()
			return
		}

		candidate := json.RawMessage(`{"entities":[],"world_entries":[],"review_issues":[]}`)
		resultHash, hashErr := contract.CanonicalHash(candidate)
		if hashErr != nil {
			serverErrors <- hashErr
			http.Error(response, "hash failed", http.StatusInternalServerError)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		if encodeErr := json.NewEncoder(response).Encode(contract.Result{
			InvocationID: invocation.InvocationID, Kind: invocation.Kind, InputHash: invocation.InputHash,
			Status: "succeeded", SchemaVersion: contract.SchemaVersion, Candidate: candidate, ResultHash: &resultHash,
			Executor: contract.Executor{Name: "process-restart-test", Version: "1", Model: "deterministic"},
		}); encodeErr != nil {
			serverErrors <- encodeErr
		}
	}))
	t.Cleanup(agentServer.Close)

	first, firstOutput := startWorkflowWorkerProcess(t, databaseURL, agentServer.URL)
	firstRequest := awaitInvocation(t, requests, serverErrors, first, firstOutput)
	if firstRequest.InvocationID != fixture.invocationID.String() {
		t.Fatalf("first process claimed invocation %s, want %s", firstRequest.InvocationID, fixture.invocationID)
	}
	firstClaim := loadInvocation()
	if firstClaim.Status != "running" || firstClaim.ClaimVersion != fixture.claimVersion+1 || firstClaim.LeaseExpiresAt == nil {
		t.Fatalf("first process did not persist its claim: %#v", firstClaim)
	}
	stopWorkflowWorkerProcess(t, first, firstOutput)

	second, secondOutput := startWorkflowWorkerProcess(t, databaseURL, agentServer.URL)
	secondRequest := awaitInvocation(t, requests, serverErrors, second, secondOutput)
	if secondRequest.InvocationID != firstRequest.InvocationID {
		t.Fatalf("restart created or claimed a different invocation: first=%s second=%s", firstRequest.InvocationID, secondRequest.InvocationID)
	}

	secondClaim := loadInvocation()
	if secondClaim.Status != "running" || secondClaim.ClaimVersion != firstClaim.ClaimVersion+1 {
		t.Fatalf("restarted process did not advance the claim fence: %#v", secondClaim)
	}
	waitForInvocationStatus(t, loadInvocation, "succeeded", second, secondOutput)
	stopWorkflowWorkerProcess(t, second, secondOutput)

	finalized := loadInvocation()
	if finalized.Attempts != fixture.claimVersion+2 || finalized.ClaimVersion != fixture.claimVersion+2 || finalized.LeaseExpiresAt != nil {
		t.Fatalf("restart did not finalize exactly one recovered claim: %#v", finalized)
	}
	var bible model.ProductionBible
	if err = database.First(&bible, "id = ?", fixture.bibleID).Error; err != nil {
		t.Fatalf("reload bible after worker restart: %v", err)
	}
	if bible.Status != "needs_review" || bible.ResultHash == nil {
		t.Fatalf("recovered worker did not persist the candidate: %#v", bible)
	}
	if requestCount.Load() != 2 {
		t.Fatalf("expected one interrupted and one recovered Agent request, got %d", requestCount.Load())
	}
}

func TestWorkflowWorkerProcessHelper(t *testing.T) {
	if os.Getenv(workerProcessHelperEnv) != "1" {
		t.Skip("subprocess helper")
	}

	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	agentURL := os.Getenv(workerProcessAgentEnv)
	secret := os.Getenv(workerProcessSecretEnv)
	database, err := platformdatabase.Open(context.Background(), databaseURL, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = platformdatabase.Close(database) }()
	signer, err := grant.NewSigner(secret, func() time.Time { return time.Now().UTC() })
	if err != nil {
		t.Fatal(err)
	}
	agentRuntime, err := client.New(agentURL, signer, nil)
	if err != nil {
		t.Fatal(err)
	}
	worker := bibleapp.NewWorker(
		biblegorm.New(database), agentRuntime, func() time.Time { return time.Now().UTC() },
		20*time.Millisecond, 500*time.Millisecond, slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	worker.Run(context.Background())
}

func startWorkflowWorkerProcess(t *testing.T, databaseURL, agentURL string) (*exec.Cmd, *bytes.Buffer) {
	t.Helper()
	output := &bytes.Buffer{}
	command := exec.Command(os.Args[0], "-test.run=^TestWorkflowWorkerProcessHelper$", "-test.v")
	command.Env = append(os.Environ(),
		workerProcessHelperEnv+"=1",
		"LANVERSE_TEST_DATABASE_URL="+databaseURL,
		workerProcessAgentEnv+"="+agentURL,
		workerProcessSecretEnv+"="+workerProcessSecret,
	)
	command.Stdout, command.Stderr = output, output
	if err := command.Start(); err != nil {
		t.Fatalf("start workflow worker process: %v", err)
	}
	t.Cleanup(func() {
		if command.ProcessState == nil && command.Process != nil {
			_ = command.Process.Kill()
			_, _ = command.Process.Wait()
		}
	})
	return command, output
}

func stopWorkflowWorkerProcess(t *testing.T, command *exec.Cmd, output *bytes.Buffer) {
	t.Helper()
	if err := command.Process.Kill(); err != nil {
		t.Fatalf("kill workflow worker process: %v\n%s", err, output.String())
	}
	if err := command.Wait(); err == nil {
		t.Fatalf("workflow worker process exited without the injected kill\n%s", output.String())
	}
}

func awaitInvocation(
	t *testing.T,
	requests <-chan contract.Invocation,
	serverErrors <-chan error,
	command *exec.Cmd,
	output *bytes.Buffer,
) contract.Invocation {
	t.Helper()
	select {
	case invocation := <-requests:
		return invocation
	case err := <-serverErrors:
		t.Fatalf("Agent test server failed: %v", err)
	case <-time.After(10 * time.Second):
		_ = command.Process.Kill()
		_, _ = command.Process.Wait()
		t.Fatalf("workflow worker process did not invoke Agent\n%s", output.String())
	}
	return contract.Invocation{}
}

func waitForInvocationStatus(
	t *testing.T,
	load func() model.AgentInvocation,
	want string,
	command *exec.Cmd,
	output *bytes.Buffer,
) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if invocation := load(); invocation.Status == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = command.Process.Kill()
	_, _ = command.Process.Wait()
	t.Fatalf("workflow worker process did not reach %s\n%s", want, output.String())
}

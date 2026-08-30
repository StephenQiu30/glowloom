package generation_test

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	costgorm "github.com/StephenQiu30/lanverse/backend/internal/cost/adapter/gormdb"
	costapp "github.com/StephenQiu30/lanverse/backend/internal/cost/application"
	costdomain "github.com/StephenQiu30/lanverse/backend/internal/cost/domain"
	generationgorm "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/gormdb"
	providersecret "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/secretstore"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	generationdomain "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	quotagorm "github.com/StephenQiu30/lanverse/backend/internal/quota/adapter/gormdb"
	quotaapp "github.com/StephenQiu30/lanverse/backend/internal/quota/application"
	quotadomain "github.com/StephenQiu30/lanverse/backend/internal/quota/domain"
	generationtestgorm "github.com/StephenQiu30/lanverse/backend/tests/generation/adapter/gormdb"
	"github.com/google/uuid"
)

type providerFencingHarness struct {
	ctx          context.Context
	database     *generationtestgorm.Database
	fixture      preparationFixture
	preparations *generationapp.PreparationService
	costConfig   costapp.Config
	quotaConfig  quotaapp.Config
}

type providerReceiptLookupGate struct {
	delegate generationapp.ProviderTransactionManager
	key      string
	observed chan<- struct{}
	release  <-chan struct{}
}

type providerReceiptLookupRepository struct {
	generationapp.ProviderRepository
	gate *providerReceiptLookupGate
}

type providerQuotaConsumeFailureTransactions struct {
	delegate generationapp.ProviderTransactionManager
	mu       sync.Mutex
	failures int
	failure  error
}

type providerQuotaConsumeFailureOwner struct {
	generationapp.QuotaProviderOwner
	transactions *providerQuotaConsumeFailureTransactions
}

type providerFencingClock struct {
	mu  sync.RWMutex
	now time.Time
}

func (clock *providerFencingClock) Now() time.Time {
	clock.mu.RLock()
	defer clock.mu.RUnlock()
	return clock.now
}

func (clock *providerFencingClock) Set(now time.Time) {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	clock.now = now
}

func (gate *providerReceiptLookupGate) WithinProviderTransaction(
	ctx context.Context,
	operation func(generationapp.ProviderRepository, generationapp.CostProviderOwner, generationapp.QuotaProviderOwner) error,
) error {
	return gate.delegate.WithinProviderTransaction(ctx, func(
		repository generationapp.ProviderRepository,
		costs generationapp.CostProviderOwner,
		quotas generationapp.QuotaProviderOwner,
	) error {
		return operation(&providerReceiptLookupRepository{ProviderRepository: repository, gate: gate}, costs, quotas)
	})
}

func (repository *providerReceiptLookupRepository) FindReceipt(
	ctx context.Context,
	workspaceID, operation, key string,
) (platformcommand.Receipt, error) {
	receipt, err := repository.ProviderRepository.FindReceipt(ctx, workspaceID, operation, key)
	if operation != "generation.provider.submit" || key != repository.gate.key ||
		!errors.Is(err, platformcommand.ErrReceiptNotFound) {
		return receipt, err
	}
	select {
	case repository.gate.observed <- struct{}{}:
	case <-ctx.Done():
		return platformcommand.Receipt{}, ctx.Err()
	}
	select {
	case <-repository.gate.release:
		return receipt, err
	case <-ctx.Done():
		return platformcommand.Receipt{}, ctx.Err()
	}
}

func (transactions *providerQuotaConsumeFailureTransactions) WithinProviderTransaction(
	ctx context.Context,
	operation func(generationapp.ProviderRepository, generationapp.CostProviderOwner, generationapp.QuotaProviderOwner) error,
) error {
	return transactions.delegate.WithinProviderTransaction(ctx, func(
		repository generationapp.ProviderRepository,
		costs generationapp.CostProviderOwner,
		quotas generationapp.QuotaProviderOwner,
	) error {
		return operation(repository, costs, &providerQuotaConsumeFailureOwner{
			QuotaProviderOwner: quotas,
			transactions:       transactions,
		})
	})
}

func (owner *providerQuotaConsumeFailureOwner) Consume(
	ctx context.Context,
	actor quotaapp.Actor,
	command quotaapp.TransitionCommand,
) (quotaapp.ReservationResult, error) {
	owner.transactions.mu.Lock()
	if owner.transactions.failures > 0 {
		owner.transactions.failures--
		failure := owner.transactions.failure
		owner.transactions.mu.Unlock()
		return quotaapp.ReservationResult{}, failure
	}
	owner.transactions.mu.Unlock()
	return owner.QuotaProviderOwner.Consume(ctx, actor, command)
}

func TestProviderRequestCreationWaitsForWorkspaceConfigurationFence(t *testing.T) {
	now := time.Date(2026, time.August, 30, 17, 0, 0, 0, time.UTC)
	harness := newProviderFencingHarness(t, now, "provider-config-fence")
	claim := prepareAndClaimProviderIntent(
		t,
		harness.ctx,
		harness.preparations,
		harness.fixture,
		"config-fence",
		strings.Repeat("4", 64),
	)

	registry, err := generationapp.NewMediaFactoryRegistry(nil)
	if err != nil {
		t.Fatalf("create empty Provider registry: %v", err)
	}
	catalog, err := generationapp.NewMediaPresetCatalog(generationdomain.MediaPresets{}, registry)
	if err != nil {
		t.Fatalf("create empty Provider catalog: %v", err)
	}
	configurationGate := &gatedProviderConfigurationTransactions{
		delegate:         generationgorm.NewProviderConfigurationStore(harness.database),
		afterOwnerLock:   make(chan struct{}),
		releaseOwnerLock: make(chan struct{}),
	}
	configuration := generationapp.NewProviderConfigurationService(
		configurationGate,
		catalog,
		providersecret.Open(""),
		generationapp.ProviderConfigurationConfig{Now: func() time.Time { return now.Add(time.Minute) }, NewID: uuid.NewString},
	)
	type configurationResult struct {
		result generationapp.ProviderConnectionResult
		err    error
	}
	disabled := make(chan configurationResult, 1)
	go func() {
		result, disableErr := configuration.SetConnectionState(
			harness.ctx,
			harness.fixture.owner,
			generationapp.SetProviderConnectionStateCommand{
				WorkspaceID:         harness.fixture.workspaceID.String(),
				ConnectionKey:       harness.fixture.provider.Connection.ConnectionKey,
				State:               generationdomain.ProviderStateDisabled,
				ExpectedRevision:    harness.fixture.provider.Connection.Revision,
				ExpectedContentHash: harness.fixture.provider.Connection.ContentHash,
				IdempotencyKey:      "provider-disable-before-request",
			},
		)
		disabled <- configurationResult{result: result, err: disableErr}
	}()
	waitProviderConfigurationSignal(t, configurationGate.afterOwnerLock, "Provider configuration Workspace fence")

	gateway := &controlledProviderGateway{submitOutcomes: map[int]generationapp.ProviderOutcome{
		1: providerSuccess("must-not-dispatch", "must-not-dispatch", strings.Repeat("f", 64)),
	}}
	providers := generationapp.NewProviderService(
		generationgorm.NewProviderStore(harness.database, harness.costConfig, harness.quotaConfig),
		gateway,
		generationapp.ProviderConfig{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	type submissionResult struct {
		result generationapp.ProviderExecutionResult
		err    error
	}
	submitted := make(chan submissionResult, 1)
	go func() {
		result, submitErr := providers.SubmitImageRequest(
			harness.ctx,
			claim.Authorization,
			generationapp.SubmitImageRequestCommand{
				IntentID: claim.Intent.ID, IdempotencyKey: "provider-submit-after-disable",
			},
		)
		submitted <- submissionResult{result: result, err: submitErr}
	}()

	select {
	case value := <-submitted:
		t.Fatalf("Provider request bypassed the held Workspace fence: result=%#v err=%v", value.result, value.err)
	case <-time.After(150 * time.Millisecond):
	}
	if preflight, submit, query := gateway.counts(); preflight != 0 || submit != 0 || query != 0 {
		t.Fatalf("Provider crossed its remote boundary while configuration held the Workspace fence: %d/%d/%d",
			preflight, submit, query)
	}
	close(configurationGate.releaseOwnerLock)

	select {
	case value := <-disabled:
		if value.err != nil || value.result.Connection.State != generationdomain.ProviderStateDisabled ||
			value.result.Connection.Revision != harness.fixture.provider.Connection.Revision+1 {
			t.Fatalf("disable Provider connection before request creation: result=%#v err=%v", value.result, value.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Provider configuration did not release its Workspace fence")
	}
	select {
	case value := <-submitted:
		if generationErrorCode(value.err) != "state_conflict" {
			t.Fatalf("Provider request accepted a disabled latest connection: result=%#v err=%v", value.result, value.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Provider request did not finish after the Workspace fence was released")
	}
	assertNoProviderExecutionFacts(t, harness.database, claim.Intent.ID)
	if preflight, submit, query := gateway.counts(); preflight != 0 || submit != 0 || query != 0 {
		t.Fatalf("configuration drift crossed the Provider boundary: %d/%d/%d", preflight, submit, query)
	}
}

func TestConcurrentProviderSubmissionsClaimOneGlobalIdempotencyFenceBeforeRemoteCall(t *testing.T) {
	now := time.Date(2026, time.August, 30, 17, 30, 0, 0, time.UTC)
	harness := newProviderFencingHarness(t, now, "provider-idempotency-fence")
	claims := []generationapp.ExecutionClaimResult{
		prepareAndClaimProviderIntent(t, harness.ctx, harness.preparations, harness.fixture,
			"idempotency-a", strings.Repeat("5", 64)),
		prepareAndClaimProviderIntent(t, harness.ctx, harness.preparations, harness.fixture,
			"idempotency-b", strings.Repeat("6", 64)),
	}
	const sharedKey = "provider-submit-shared-global-key"
	lookupObserved, releaseLookup := make(chan struct{}, 2), make(chan struct{})
	transactions := &providerReceiptLookupGate{
		delegate: generationgorm.NewProviderStore(harness.database, harness.costConfig, harness.quotaConfig),
		key:      sharedKey, observed: lookupObserved, release: releaseLookup,
	}
	submitStarted, releaseSubmit := make(chan struct{}, 1), make(chan struct{})
	gateway := &controlledProviderGateway{
		submitOutcomes: map[int]generationapp.ProviderOutcome{
			1: providerSuccess("idempotency-winner", "idempotency-winner", strings.Repeat("e", 64)),
		},
		submitStarted: submitStarted,
		releaseSubmit: releaseSubmit,
	}
	providers := generationapp.NewProviderService(
		transactions,
		gateway,
		generationapp.ProviderConfig{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	type namedSubmissionResult struct {
		index  int
		result generationapp.ProviderExecutionResult
		err    error
	}
	results := make(chan namedSubmissionResult, 2)
	start := make(chan struct{})
	for index := range claims {
		index := index
		go func() {
			<-start
			result, submitErr := providers.SubmitImageRequest(
				harness.ctx,
				claims[index].Authorization,
				generationapp.SubmitImageRequestCommand{IntentID: claims[index].Intent.ID, IdempotencyKey: sharedKey},
			)
			results <- namedSubmissionResult{index: index, result: result, err: submitErr}
		}()
	}
	close(start)
	for index := 0; index < len(claims); index++ {
		select {
		case <-lookupObserved:
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent Provider submissions did not both observe the pre-claim receipt gap")
		}
	}
	close(releaseLookup)
	select {
	case <-submitStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("winning Provider submission did not cross the remote boundary")
	}

	var loser namedSubmissionResult
	select {
	case loser = <-results:
		if generationErrorCode(loser.err) != "state_conflict" {
			t.Fatalf("losing Provider submission did not reject the shared key: result=%#v err=%v", loser.result, loser.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("losing Provider submission did not fail before the winner returned from remote")
	}
	winnerIndex := 1 - loser.index
	assertPendingProviderExecution(t, harness.database, claims[loser.index].Intent.ID)
	if _, submit, _ := gateway.counts(); submit != 1 {
		t.Fatalf("shared Provider idempotency key made %d remote submissions before release, want 1", submit)
	}

	sameKeyReplay, err := providers.SubmitImageRequest(
		harness.ctx,
		claims[winnerIndex].Authorization,
		generationapp.SubmitImageRequestCommand{IntentID: claims[winnerIndex].Intent.ID, IdempotencyKey: sharedKey},
	)
	if err != nil || sameKeyReplay.Calls[0].Status != generationdomain.ProviderCallDispatching || sameKeyReplay.Receipt.ID == "" {
		t.Fatalf("same-key Provider follower did not replay the dispatch fence: result=%#v err=%v", sameKeyReplay, err)
	}
	differentKeyFollower, err := providers.SubmitImageRequest(
		harness.ctx,
		claims[winnerIndex].Authorization,
		generationapp.SubmitImageRequestCommand{
			IntentID: claims[winnerIndex].Intent.ID, IdempotencyKey: "provider-submit-dispatching-follower",
		},
	)
	if err != nil || differentKeyFollower.Calls[0].Status != generationdomain.ProviderCallDispatching {
		t.Fatalf("different-key Provider follower crossed the dispatch fence: result=%#v err=%v", differentKeyFollower, err)
	}
	if _, submit, _ := gateway.counts(); submit != 1 {
		t.Fatalf("Provider followers made %d remote submissions while the first was in flight", submit)
	}

	close(releaseSubmit)
	select {
	case winner := <-results:
		if winner.index != winnerIndex || winner.err != nil ||
			winner.result.Calls[0].Status != generationdomain.ProviderCallSucceeded {
			t.Fatalf("winning Provider submission did not persist its terminal result: %#v", winner)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("winning Provider submission did not finish after remote release")
	}
	if _, submit, _ := gateway.counts(); submit != 1 {
		t.Fatalf("global Provider idempotency fence allowed %d remote submissions, want 1", submit)
	}
}

func TestProviderTerminalQueryAtDeadlineIsFencedByPersistedCallState(t *testing.T) {
	startedAt := time.Date(2026, time.August, 30, 18, 0, 0, 0, time.UTC)
	harness := newProviderFencingHarness(t, startedAt, "provider-terminal-deadline-fence")

	for _, testCase := range []struct {
		name        string
		expireFirst bool
	}{
		{name: "terminal_query_commits_first"},
		{name: "expiration_commits_first", expireFirst: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			deadlineAt := startedAt.Add(2 * time.Hour)
			remoteExpiresAt := startedAt.Add(26 * time.Hour)
			clock := &providerFencingClock{now: startedAt}
			queryStarted, releaseQuery := make(chan struct{}, 1), make(chan struct{})
			gateway := &controlledProviderGateway{
				preflightFailures: map[int]error{
					1: controlledProviderFailure("provider.invalid_candidate_1"),
					2: controlledProviderFailure("provider.invalid_candidate_2"),
					3: controlledProviderFailure("provider.invalid_candidate_3"),
				},
				submitOutcomes: map[int]generationapp.ProviderOutcome{
					4: {
						Status: generationapp.ProviderOutcomeAccepted, RemoteRequestID: "deadline-request-" + testCase.name,
						RemoteJobID: "deadline-job-" + testCase.name, QueryDeadlineAt: deadlineAt,
						RemoteExpiresAt: remoteExpiresAt,
					},
				},
				queryOutcomes: map[int][]generationapp.ProviderOutcome{
					4: {providerSuccess("deadline-output-"+testCase.name,
						"deadline-event-"+testCase.name, strings.Repeat("d", 64))},
				},
				queryStarted: queryStarted,
				releaseQuery: releaseQuery,
			}
			providers := generationapp.NewProviderService(
				generationgorm.NewProviderStore(harness.database, harness.costConfig, harness.quotaConfig),
				gateway,
				generationapp.ProviderConfig{Now: clock.Now, NewID: uuid.NewString},
			)
			claim := prepareAndClaimProviderIntent(
				t,
				harness.ctx,
				harness.preparations,
				harness.fixture,
				"deadline-fence-"+testCase.name,
				strings.Repeat("7", 64),
			)
			var submitted generationapp.ProviderExecutionResult
			for candidate := 1; candidate <= 4; candidate++ {
				submitted = submitProvider(
					t,
					harness.ctx,
					providers,
					claim,
					"provider-deadline-fence-"+testCase.name+"-submit-"+string(rune('0'+candidate)),
				)
			}
			if submitted.Calls[3].Status != generationdomain.ProviderCallSubmitted ||
				submitted.Calls[3].QueryDeadlineAt == nil || !submitted.Calls[3].QueryDeadlineAt.Equal(deadlineAt) ||
				submitted.Calls[3].RemoteExpiresAt == nil || !submitted.Calls[3].RemoteExpiresAt.Equal(remoteExpiresAt) {
				t.Fatalf("persist asynchronous Provider deadline: %#v", submitted.Calls[3])
			}

			clock.Set(deadlineAt.Add(-time.Microsecond))
			type reconciliationResult struct {
				result generationapp.ProviderExecutionResult
				err    error
			}
			queried := make(chan reconciliationResult, 1)
			go func() {
				result, queryErr := providers.ReconcileProviderJob(
					harness.ctx,
					generationapp.ReconcileProviderJobCommand{
						ProviderJobID:  submitted.Job.ID,
						IdempotencyKey: "provider-deadline-fence-query-" + testCase.name,
					},
				)
				queried <- reconciliationResult{result: result, err: queryErr}
			}()
			select {
			case <-queryStarted:
			case <-time.After(5 * time.Second):
				t.Fatal("Provider query did not start before its persisted deadline")
			}
			clock.Set(deadlineAt)

			if testCase.expireFirst {
				expired := reconcileProvider(
					t,
					harness.ctx,
					providers,
					submitted.Job.ID,
					"provider-deadline-fence-expire-"+testCase.name,
				)
				if expired.Job.Status != generationdomain.ProviderJobOutcomeUnknown ||
					expired.Calls[3].Status != generationdomain.ProviderCallOutcomeUnknown || len(expired.Receipts) != 0 {
					t.Fatalf("deadline winner did not persist OUTCOME_UNKNOWN: %#v", expired)
				}
			}
			close(releaseQuery)
			select {
			case value := <-queried:
				if value.err != nil {
					t.Fatalf("apply Provider query racing its deadline: %v", value.err)
				}
				if testCase.expireFirst {
					if value.result.Job.Status != generationdomain.ProviderJobOutcomeUnknown ||
						value.result.Calls[3].Status != generationdomain.ProviderCallOutcomeUnknown ||
						len(value.result.Receipts) != 0 {
						t.Fatalf("late terminal Provider result overwrote OUTCOME_UNKNOWN: %#v", value.result)
					}
				} else if value.result.Job.Status != generationdomain.ProviderJobPartialSucceeded ||
					value.result.Calls[3].Status != generationdomain.ProviderCallSucceeded ||
					value.result.Calls[3].QueryDeadlineAt == nil ||
					!value.result.Calls[3].QueryDeadlineAt.Equal(deadlineAt) ||
					value.result.Calls[3].RemoteExpiresAt == nil ||
					!value.result.Calls[3].RemoteExpiresAt.Equal(remoteExpiresAt) || len(value.result.Receipts) != 1 {
					t.Fatalf("confirmed terminal Provider result was downgraded at its deadline: %#v", value.result)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Provider query did not leave the deadline race")
			}
			if _, _, queries := gateway.counts(); queries != 1 {
				t.Fatalf("Provider deadline race made %d queries, want 1", queries)
			}
		})
	}
}

func TestProviderRemoteIdentityCannotBeHiddenByMalformedOutcomeFields(t *testing.T) {
	now := time.Date(2026, time.August, 30, 18, 30, 0, 0, time.UTC)
	harness := newProviderFencingHarness(t, now, "provider-raw-identity-fence")
	deadlineAt := now.Add(2 * time.Hour)
	remoteExpiresAt := now.Add(26 * time.Hour)
	gateway := &controlledProviderGateway{
		preflightFailures: map[int]error{
			1: controlledProviderFailure("provider.invalid_candidate_1"),
			2: controlledProviderFailure("provider.invalid_candidate_2"),
			3: controlledProviderFailure("provider.invalid_candidate_3"),
		},
		submitOutcomes: map[int]generationapp.ProviderOutcome{
			4: {
				Status: generationapp.ProviderOutcomeAccepted, RemoteRequestID: "raw-identity-request-a",
				RemoteJobID: "raw-identity-job-a", QueryDeadlineAt: deadlineAt,
				RemoteExpiresAt: remoteExpiresAt,
			},
		},
		queryOutcomes: map[int][]generationapp.ProviderOutcome{
			4: {
				{
					Status: generationapp.ProviderOutcomeRunning, RemoteRequestID: "raw-identity-request-a",
					RemoteJobID: "raw-identity-job-b", QueryDeadlineAt: deadlineAt.Add(time.Hour),
					RemoteExpiresAt:          remoteExpiresAt.Add(time.Hour),
					ProviderUsageObservation: generationdomain.ProviderUsageObservation{InputTokens: -1},
				},
				providerSuccess("raw-identity-output", "raw-identity-event", strings.Repeat("c", 64)),
			},
		},
	}
	providers := generationapp.NewProviderService(
		generationgorm.NewProviderStore(harness.database, harness.costConfig, harness.quotaConfig),
		gateway,
		generationapp.ProviderConfig{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	claim := prepareAndClaimProviderIntent(
		t,
		harness.ctx,
		harness.preparations,
		harness.fixture,
		"raw-identity-fence",
		strings.Repeat("8", 64),
	)
	var submitted generationapp.ProviderExecutionResult
	for candidate := 1; candidate <= 4; candidate++ {
		submitted = submitProvider(
			t,
			harness.ctx,
			providers,
			claim,
			"provider-raw-identity-submit-"+string(rune('0'+candidate)),
		)
	}
	if submitted.Calls[3].Status != generationdomain.ProviderCallSubmitted ||
		submitted.Calls[3].RemoteJobID != "raw-identity-job-a" {
		t.Fatalf("persist original Provider remote identity: %#v", submitted.Calls[3])
	}
	if _, err := providers.ReconcileProviderJob(
		harness.ctx,
		generationapp.ReconcileProviderJobCommand{
			ProviderJobID: submitted.Job.ID, IdempotencyKey: "provider-raw-identity-malformed-query",
		},
	); generationErrorCode(err) != "state_conflict" {
		t.Fatalf("malformed outcome hid Provider remote identity drift: %v", err)
	}
	var persisted model.GenerationProviderCall
	if err := harness.database.First(&persisted, "id = ?", submitted.Calls[3].ID).Error; err != nil {
		t.Fatalf("load Provider Call after rejected identity drift: %v", err)
	}
	if persisted.Status != generationdomain.ProviderCallSubmitted || persisted.RemoteJobID == nil ||
		*persisted.RemoteJobID != "raw-identity-job-a" || persisted.QueryDeadlineAt == nil ||
		!persisted.QueryDeadlineAt.Equal(deadlineAt) || persisted.RemoteExpiresAt == nil ||
		!persisted.RemoteExpiresAt.Equal(remoteExpiresAt) {
		t.Fatalf("rejected Provider identity drift changed the persisted binding: %#v", persisted)
	}
	if _, submits, queries := gateway.counts(); submits != 1 || queries != 1 {
		t.Fatalf("rejected Provider identity drift crossed unexpected boundaries: submit=%d query=%d", submits, queries)
	}
	recovered := reconcileProvider(
		t,
		harness.ctx,
		providers,
		submitted.Job.ID,
		"provider-raw-identity-recover-query",
	)
	if recovered.Job.Status != generationdomain.ProviderJobPartialSucceeded ||
		recovered.Calls[3].Status != generationdomain.ProviderCallSucceeded {
		t.Fatalf("Provider Call did not recover after rejecting identity drift: %#v", recovered)
	}
	if _, submits, queries := gateway.counts(); submits != 1 || queries != 2 {
		t.Fatalf("Provider identity recovery boundaries: submit=%d query=%d, want 1/2", submits, queries)
	}
}

func TestProviderSubmitErrorWithOfficialRemoteIdentityRemainsQueryable(t *testing.T) {
	now := time.Date(2026, time.August, 30, 19, 0, 0, 0, time.UTC)
	harness := newProviderFencingHarness(t, now, "provider-submit-identity-recovery")
	deadlineAt := now.Add(2 * time.Hour)
	remoteExpiresAt := now.Add(26 * time.Hour)
	gateway := &controlledProviderGateway{
		preflightFailures: map[int]error{
			1: controlledProviderFailure("provider.invalid_candidate_1"),
			2: controlledProviderFailure("provider.invalid_candidate_2"),
			3: controlledProviderFailure("provider.invalid_candidate_3"),
		},
		submitOutcomes: map[int]generationapp.ProviderOutcome{
			4: {
				Status: generationapp.ProviderOutcomeUnknown, RemoteRequestID: "submit-error-request",
				RemoteJobID: "submit-error-job", QueryDeadlineAt: deadlineAt,
				RemoteExpiresAt: remoteExpiresAt,
			},
		},
		submitFailures: map[int]error{
			4: controlledProviderSubmitFailure{
				kind:    generationapp.ProviderSubmitFailureIdentityRecoverable,
				message: "response interrupted after the official task identity was decoded",
			},
		},
		queryOutcomes: map[int][]generationapp.ProviderOutcome{
			4: {providerSuccess("submit-error-output", "submit-error-event", strings.Repeat("b", 64))},
		},
	}
	providers := generationapp.NewProviderService(
		generationgorm.NewProviderStore(harness.database, harness.costConfig, harness.quotaConfig),
		gateway,
		generationapp.ProviderConfig{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	claim := prepareAndClaimProviderIntent(
		t,
		harness.ctx,
		harness.preparations,
		harness.fixture,
		"submit-identity-recovery",
		strings.Repeat("9", 64),
	)
	var submitted generationapp.ProviderExecutionResult
	for candidate := 1; candidate <= 4; candidate++ {
		submitted = submitProvider(
			t,
			harness.ctx,
			providers,
			claim,
			"provider-submit-identity-recovery-"+string(rune('0'+candidate)),
		)
	}
	if submitted.Calls[3].Status != generationdomain.ProviderCallSubmitted ||
		submitted.Calls[3].RemoteRequestID != "submit-error-request" ||
		submitted.Calls[3].RemoteJobID != "submit-error-job" ||
		submitted.Calls[3].QueryDeadlineAt == nil || !submitted.Calls[3].QueryDeadlineAt.Equal(deadlineAt) ||
		submitted.Calls[3].RemoteExpiresAt == nil || !submitted.Calls[3].RemoteExpiresAt.Equal(remoteExpiresAt) {
		t.Fatalf("typed Provider Submit ambiguity lost its queryable remote identity: %#v", submitted.Calls[3])
	}
	recovered := reconcileProvider(
		t,
		harness.ctx,
		providers,
		submitted.Job.ID,
		"provider-submit-identity-recovery-query",
	)
	if recovered.Job.Status != generationdomain.ProviderJobPartialSucceeded ||
		recovered.Calls[3].Status != generationdomain.ProviderCallSucceeded {
		t.Fatalf("queryable Provider Submit ambiguity did not recover: %#v", recovered)
	}
	if _, submits, queries := gateway.counts(); submits != 1 || queries != 1 {
		t.Fatalf("Provider Submit identity recovery boundaries: submit=%d query=%d, want 1/1", submits, queries)
	}
}

func TestProviderTerminalOwnerFailureRollsBackAndRequeriesSameRemoteIdentity(t *testing.T) {
	now := time.Date(2026, time.August, 30, 19, 30, 0, 0, time.UTC)
	harness := newProviderFencingHarness(t, now, "provider-owner-rollback")
	deadlineAt := now.Add(2 * time.Hour)
	remoteExpiresAt := now.Add(26 * time.Hour)
	terminalOutcome := providerSuccess("owner-rollback-output", "owner-rollback-event", strings.Repeat("a", 64))
	gateway := &controlledProviderGateway{
		preflightFailures: map[int]error{
			1: controlledProviderFailure("provider.invalid_candidate_1"),
			2: controlledProviderFailure("provider.invalid_candidate_2"),
			3: controlledProviderFailure("provider.invalid_candidate_3"),
		},
		submitOutcomes: map[int]generationapp.ProviderOutcome{
			4: {
				Status: generationapp.ProviderOutcomeAccepted, RemoteRequestID: "owner-rollback-request",
				RemoteJobID: "owner-rollback-job", QueryDeadlineAt: deadlineAt,
				RemoteExpiresAt: remoteExpiresAt,
			},
		},
		queryOutcomes: map[int][]generationapp.ProviderOutcome{4: {terminalOutcome, terminalOutcome}},
	}
	injected := errors.New("injected quota consume failure after cost settlement")
	transactions := &providerQuotaConsumeFailureTransactions{
		delegate: generationgorm.NewProviderStore(harness.database, harness.costConfig, harness.quotaConfig),
		failures: 1,
		failure:  injected,
	}
	providers := generationapp.NewProviderService(
		transactions,
		gateway,
		generationapp.ProviderConfig{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	claim := prepareAndClaimProviderIntent(
		t,
		harness.ctx,
		harness.preparations,
		harness.fixture,
		"owner-rollback",
		strings.Repeat("a", 64),
	)
	var submitted generationapp.ProviderExecutionResult
	for candidate := 1; candidate <= 4; candidate++ {
		submitted = submitProvider(
			t,
			harness.ctx,
			providers,
			claim,
			"provider-owner-rollback-submit-"+string(rune('0'+candidate)),
		)
	}
	if submitted.Calls[3].Status != generationdomain.ProviderCallSubmitted ||
		submitted.Calls[3].RemoteJobID != "owner-rollback-job" {
		t.Fatalf("persist Provider identity before Owner rollback test: %#v", submitted.Calls[3])
	}
	const reconcileKey = "provider-owner-rollback-reconcile"
	if _, err := providers.ReconcileProviderJob(
		harness.ctx,
		generationapp.ReconcileProviderJobCommand{ProviderJobID: submitted.Job.ID, IdempotencyKey: reconcileKey},
	); !errors.Is(err, injected) {
		t.Fatalf("Provider terminal transaction returned %v, want injected Quota failure", err)
	}

	var rolledBackCall model.GenerationProviderCall
	if err := harness.database.First(&rolledBackCall, "id = ?", submitted.Calls[3].ID).Error; err != nil {
		t.Fatalf("load Provider Call after Owner rollback: %v", err)
	}
	var rolledBackJob model.GenerationProviderJob
	if err := harness.database.First(&rolledBackJob, "id = ?", submitted.Job.ID).Error; err != nil {
		t.Fatalf("load Provider Job after Owner rollback: %v", err)
	}
	if rolledBackCall.Status != generationdomain.ProviderCallSubmitted || rolledBackCall.RemoteJobID == nil ||
		*rolledBackCall.RemoteJobID != "owner-rollback-job" ||
		rolledBackJob.Status != generationdomain.ProviderJobRunning || rolledBackJob.SucceededCallCount != 0 {
		t.Fatalf("Owner failure partially committed Provider terminal facts: call=%#v job=%#v", rolledBackCall, rolledBackJob)
	}
	assertProviderReservations(
		t,
		harness.ctx,
		costapp.NewService(costgorm.New(harness.database), harness.costConfig),
		quotaapp.NewService(quotagorm.New(harness.database), harness.quotaConfig),
		harness.fixture,
		submitted.Intent,
		costdomain.ReservationReserved,
		quotadomain.ReservationReserved,
	)
	assertProviderRollbackCounts(t, harness.database, submitted, reconcileKey, 0)
	if _, submits, queries := gateway.counts(); submits != 1 || queries != 1 {
		t.Fatalf("failed Owner transaction boundaries: submit=%d query=%d, want 1/1", submits, queries)
	}

	recovered := reconcileProvider(t, harness.ctx, providers, submitted.Job.ID, reconcileKey)
	if recovered.Job.Status != generationdomain.ProviderJobPartialSucceeded ||
		recovered.Calls[3].Status != generationdomain.ProviderCallSucceeded ||
		recovered.Calls[3].RemoteJobID != "owner-rollback-job" || len(recovered.Receipts) != 1 {
		t.Fatalf("Provider Owner retry did not converge from the same remote identity: %#v", recovered)
	}
	costView, err := costapp.NewService(costgorm.New(harness.database), harness.costConfig).GetReservation(
		harness.ctx,
		costapp.Actor{UserID: harness.fixture.editor.UserID, TokenVersion: harness.fixture.editor.TokenVersion},
		recovered.Intent.CostReservationID,
	)
	if err != nil || costView.Reservation.Status != costdomain.ReservationSettled {
		t.Fatalf("Provider retry did not settle Cost exactly once: reservation=%#v err=%v", costView.Reservation, err)
	}
	quotaReservation, err := quotaapp.NewService(quotagorm.New(harness.database), harness.quotaConfig).GetReservation(
		harness.ctx,
		quotaapp.Actor{UserID: harness.fixture.editor.UserID, TokenVersion: harness.fixture.editor.TokenVersion},
		recovered.Intent.QuotaReservationID,
	)
	if err != nil || quotaReservation.Status != quotadomain.ReservationConsumed {
		t.Fatalf("Provider retry did not consume Quota exactly once: reservation=%#v err=%v", quotaReservation, err)
	}
	assertProviderRollbackCounts(t, harness.database, recovered, reconcileKey, 1)
	if _, submits, queries := gateway.counts(); submits != 1 || queries != 2 {
		t.Fatalf("Provider Owner recovery boundaries: submit=%d query=%d, want 1/2", submits, queries)
	}
}

func newProviderFencingHarness(t *testing.T, now time.Time, suffix string) providerFencingHarness {
	t.Helper()
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the Provider fencing journey")
	}
	ctx := context.Background()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatalf("open Provider fencing database: %v", err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatalf("synchronize Provider fencing GORM catalog: %v", err)
	}
	create := func(value any) error { return database.Create(value).Error }
	fixture := seedPreparationFixture(t, create, generationgorm.NewTargetStore(database), now, suffix)
	fixture.provider = seedControlledProjectProviderBinding(
		t,
		create,
		fixture,
		"controlled-image",
		"image-quality",
		1,
	)
	t.Cleanup(func() {
		cleanupProviderFixture(t, func(value any, query string, arguments ...any) error {
			return generationtestgorm.DeleteWithoutHooks(database, value, query, arguments...)
		}, fixture)
	})
	costConfig := costapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}
	quotaConfig := quotaapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}
	costs := costapp.NewService(costgorm.New(database), costConfig)
	quotas := quotaapp.NewService(quotagorm.New(database), quotaConfig)
	configurePreparationLimits(t, ctx, costs, quotas, fixture, "1000.000000", "10.000000", 100)
	preparations := generationapp.NewPreparationService(
		generationgorm.NewPreparationStore(database, costConfig, quotaConfig),
		generationapp.PreparationConfig{Now: func() time.Time { return now }, NewID: uuid.NewString, ClaimTTL: preparationClaimTTL},
	)
	return providerFencingHarness{
		ctx: ctx, database: database, fixture: fixture, preparations: preparations,
		costConfig: costConfig, quotaConfig: quotaConfig,
	}
}

func assertNoProviderExecutionFacts(t *testing.T, database *generationtestgorm.Database, intentID string) {
	t.Helper()
	checks := []struct {
		value any
		name  string
	}{
		{value: &model.GenerationRequest{}, name: "Generation requests"},
		{value: &model.GenerationProviderJob{}, name: "Provider jobs"},
	}
	for _, check := range checks {
		var count int64
		if err := database.Model(check.value).Where("intent_id = ?", intentID).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("configuration-fenced %s count = %d, want 0: %v", check.name, count, err)
		}
	}
}

func assertPendingProviderExecution(t *testing.T, database *generationtestgorm.Database, intentID string) {
	t.Helper()
	var job model.GenerationProviderJob
	if err := database.Where("intent_id = ?", intentID).First(&job).Error; err != nil {
		t.Fatalf("load losing Provider job: %v", err)
	}
	var calls []model.GenerationProviderCall
	if err := database.Where("job_id = ?", job.ID).Order("candidate_index ASC").Find(&calls).Error; err != nil {
		t.Fatalf("load losing Provider Calls: %v", err)
	}
	if job.Status != generationdomain.ProviderJobPending || len(calls) != 4 {
		t.Fatalf("losing Provider execution did not remain pending: job=%#v calls=%#v", job, calls)
	}
	for _, call := range calls {
		if call.Status != generationdomain.ProviderCallPending || call.DispatchBoundaryEnteredAt != nil {
			t.Fatalf("losing Provider Call crossed the dispatch boundary: %#v", call)
		}
	}
}

func assertProviderRollbackCounts(
	t *testing.T,
	database *generationtestgorm.Database,
	result generationapp.ProviderExecutionResult,
	reconcileKey string,
	want int64,
) {
	t.Helper()
	checks := []struct {
		value any
		query string
		args  []any
		name  string
	}{
		{
			value: &model.GenerationProviderResultReceipt{}, query: "call_id = ?", args: []any{result.Calls[3].ID},
			name: "Provider result receipts",
		},
		{
			value: &model.CommandReceipt{}, query: "workspace_id = ? AND operation = ? AND idempotency_key = ?",
			args: []any{result.Intent.WorkspaceID, "generation.provider.reconcile", reconcileKey},
			name: "Provider reconcile receipts",
		},
		{
			value: &model.CommandReceipt{}, query: "workspace_id = ? AND operation = ?",
			args: []any{result.Intent.WorkspaceID, "generation.provider.terminal"},
			name: "Provider terminal receipts",
		},
		{
			value: &model.CommandReceipt{}, query: "workspace_id = ? AND operation = ?",
			args: []any{result.Intent.WorkspaceID, "cost.reservation.settle"},
			name: "Cost settlement receipts",
		},
		{
			value: &model.CommandReceipt{}, query: "workspace_id = ? AND operation = ?",
			args: []any{result.Intent.WorkspaceID, "quota.reservation.consume"},
			name: "Quota consumption receipts",
		},
	}
	for _, check := range checks {
		var count int64
		if err := database.Model(check.value).Where(check.query, check.args...).Count(&count).Error; err != nil || count != want {
			t.Fatalf("%s after Provider Owner transition = %d, want %d: %v", check.name, count, want, err)
		}
	}
}

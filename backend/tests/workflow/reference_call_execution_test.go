package workflow_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	generationgorm "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/gormdb"
	openaiadapter "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/openai"
	app "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	generationtestgorm "github.com/StephenQiu30/lanverse/backend/tests/generation/adapter/gormdb"
	fixturegorm "github.com/StephenQiu30/lanverse/backend/tests/platform/adapter/gormdb"
	"github.com/google/uuid"
)

func assertReferenceCallExecutionPersistence(t *testing.T, ctx context.Context, database *generationtestgorm.Database, actor app.Actor, fixture referencePreparationFixture, configuration referenceExecutionFixture) {
	t.Helper()
	for _, mode := range []string{"success", "unknown", "rejected", "not_sent", "preflight", "decrypt", "commit", "race", "drift", "foreign_observation", "invalid_disposition", "cancelled"} {
		t.Run("reference_execute_"+mode, func(t *testing.T) {
			if err := database.SavePoint("reference_execute_journey").Error; err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := database.RollbackTo("reference_execute_journey").Error; err != nil {
					t.Error(err)
				}
			}()
			store := generationgorm.New(database)
			var rows []model.GenerationReferenceProviderCall
			if err := database.Where("execution_id = ?", fixture.execution.ID).Order("bundle_index, slot_key").Find(&rows).Error; err != nil || len(rows) == 0 {
				t.Fatalf("read calls: %v", err)
			}
			command := app.ClaimReferenceCallCommand{WorkspaceID: fixture.execution.WorkspaceID, ProjectID: fixture.execution.ProjectID, ExecutionRef: domain.GenerationRevisionRef{ID: fixture.execution.ID, Revision: fixture.execution.Revision, ContentHash: fixture.execution.ContentHash}, CallKey: rows[0].CallKey, ExpectedRevision: 1}
			clock := fixture.execution.CreatedAt.Add(time.Minute)
			executeCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			transactions := &referenceExecutionTrackedTransactions{base: store, failReceipt: mode == "commit"}
			secrets := &referenceExecutionTrackedSecrets{base: configuration.secrets, transactions: transactions, fail: mode == "decrypt"}
			factory := &referenceExecutionFactory{Factory: openaiadapter.NewFactory(nil, nil, nil), mode: mode, now: clock.Add(time.Second), transactions: transactions}
			registry, err := app.NewMediaFactoryRegistry([]app.MediaAdapterFactory{factory})
			if err != nil {
				t.Fatal(err)
			}
			service, err := app.NewReferenceCallExecutionService(transactions, registry, secrets, func() time.Time { return clock }, uuid.NewString)
			if err != nil {
				t.Fatal(err)
			}
			readState := func() domain.ReferenceCallState {
				t.Helper()
				var state domain.ReferenceCallState
				if err := store.WithinReferenceCallDispatch(ctx, func(repo app.ReferenceCallDispatchRepository) error {
					var err error
					state, err = repo.FindReferenceCallState(ctx, command.WorkspaceID, command.ProjectID, command.ExecutionRef.ID, command.CallKey)
					return err
				}); err != nil {
					t.Fatal(err)
				}
				return state
			}
			factory.beforeSubmit = func() {
				if readState().Status != domain.ProviderCallDispatching {
					t.Fatal("Submit ran before dispatch transaction completed")
				}
				if mode == "success" { // Completed history must survive current Head drift.
					if err := database.Model(&model.GenerationReferenceExecutionHead{}).Where("current_execution_id = ?", fixture.execution.ID).UpdateColumns(map[string]any{"current_execution_hash": strings.Repeat("f", 64)}).Error; err != nil {
						t.Fatal(err)
					}
				}
				if mode == "cancelled" {
					cancel()
				}
			}
			factory.beforePreflight = func() {
				if mode == "race" && factory.preflights == 1 {
					if result, err := service.Execute(ctx, actor, command); err != nil || result.Status != domain.ProviderCallSucceeded {
						t.Fatalf("interleaved winner failed: %v", err)
					}
				}
				if mode == "drift" {
					if err := database.Model(&model.GenerationReferenceExecutionHead{}).Where("current_execution_id = ?", fixture.execution.ID).UpdateColumns(map[string]any{"current_execution_hash": strings.Repeat("f", 64)}).Error; err != nil {
						t.Fatal(err)
					}
				}
			}
			result, err := service.Execute(executeCtx, actor, command)
			if mode == "preflight" || mode == "decrypt" || mode == "commit" || mode == "drift" || mode == "foreign_observation" || mode == "invalid_disposition" || mode == "cancelled" {
				if err == nil || !reflect.DeepEqual(result, domain.ReferenceCallState{}) {
					t.Fatalf("failure leaked result: %+v %v", result, err)
				}
				state := readState()
				expected, sends := domain.ProviderCallPending, 0
				if mode == "commit" || mode == "foreign_observation" || mode == "invalid_disposition" || mode == "cancelled" {
					expected, sends = domain.ProviderCallDispatching, 1
				}
				if state.Status != expected || state.Receipt != nil || factory.submits != sends {
					t.Fatalf("failure left state=%+v sends=%d", state, factory.submits)
				}
				if sends == 1 {
					replay, replayErr := service.Execute(ctx, actor, command)
					if replayErr != nil || !reflect.DeepEqual(replay, state) || factory.submits != 1 {
						t.Fatal("failed receipt caused another Submit")
					}
				}
			} else {
				expected := domain.ProviderCallSucceeded
				if mode == "unknown" {
					expected = domain.ProviderCallOutcomeUnknown
				}
				if mode == "rejected" || mode == "not_sent" {
					expected = domain.ProviderCallFailed
				}
				if err != nil || result.Status != expected || result.Receipt == nil || factory.submits != 1 {
					t.Fatalf("execute: %+v err=%v submits=%d", result, err, factory.submits)
				}
				if !reflect.DeepEqual(readState(), result) {
					t.Fatal("receipt and state were not persisted together")
				}
				calls := secrets.calls
				if replay, err := service.Execute(ctx, actor, command); err != nil || !reflect.DeepEqual(replay, result) || secrets.calls != calls || factory.submits != 1 {
					t.Fatal("replay re-entered provider runtime")
				}
				if err := store.WithinReferenceCallDispatch(ctx, func(repo app.ReferenceCallDispatchRepository) error {
					usage, err := repo.ReferenceCallDispatchUsage(ctx, command.WorkspaceID, clock.Truncate(24*time.Hour), clock.Truncate(24*time.Hour).Add(24*time.Hour))
					unresolved := int64(0)
					if mode == "unknown" {
						unresolved = 1
					}
					if err == nil && (usage.Unresolved != unresolved || usage.Daily != 1) {
						t.Errorf("incorrect receipt accounting: %+v", usage)
					}
					return err
				}); err != nil {
					t.Fatal(err)
				}
			}
			for _, plaintext := range factory.borrowedSecrets {
				for _, b := range plaintext {
					if b != 0 {
						t.Fatal("invocation plaintext was retained")
					}
				}
			}
			for _, b := range secrets.failedPlaintext {
				if b != 0 {
					t.Fatal("partial failed decryption was not wiped")
				}
			}
		})
	}
}

type referenceExecutionTrackedTransactions struct {
	base        app.ReferenceCallDispatchTransactions
	depth       int
	failReceipt bool
}

// This fault-injection double intentionally uses the outer fixture savepoint.
// It proves application control flow only, not durability. The standalone test
// below uses the production transaction port and a second connection at send time.
func (tx *referenceExecutionTrackedTransactions) WithinReferenceCallExecution(ctx context.Context, operation func(app.ReferenceCallDispatchRepository) error) error {
	return tx.WithinReferenceCallDispatch(ctx, operation)
}

func (tx *referenceExecutionTrackedTransactions) WithinReferenceCallDispatch(ctx context.Context, operation func(app.ReferenceCallDispatchRepository) error) error {
	tx.depth++
	defer func() { tx.depth-- }()
	return tx.base.WithinReferenceCallDispatch(ctx, func(repo app.ReferenceCallDispatchRepository) error {
		wrapped := &referenceExecutionReceiptFailureRepository{ReferenceCallDispatchRepository: repo}
		if err := operation(wrapped); err != nil {
			return err
		}
		if tx.failReceipt && wrapped.wroteReceipt {
			return errors.New("injected receipt transaction failure")
		}
		return nil
	})
}

type referenceExecutionReceiptFailureRepository struct {
	app.ReferenceCallDispatchRepository
	wroteReceipt bool
}

func (repo *referenceExecutionReceiptFailureRepository) UpdateReferenceCallState(ctx context.Context, workspace, project, execution string, before, after domain.ReferenceCallState) error {
	if err := repo.ReferenceCallDispatchRepository.UpdateReferenceCallState(ctx, workspace, project, execution, before, after); err != nil {
		return err
	}
	repo.wroteReceipt = after.Receipt != nil
	return nil
}

type referenceExecutionTrackedSecrets struct {
	base            app.ProviderRuntimeSecrets
	transactions    *referenceExecutionTrackedTransactions
	fail            bool
	calls           int
	failedPlaintext []byte
}

func (secrets *referenceExecutionTrackedSecrets) Decrypt(ctx context.Context, scope domain.ProviderSecretContext, encrypted domain.EncryptedProviderSecret) ([]byte, error) {
	secrets.calls++
	if secrets.fail {
		secrets.failedPlaintext = []byte("synthetic partial plaintext")
		return secrets.failedPlaintext, errors.New("decryption failed")
	}
	if secrets.transactions.depth != 0 {
		return nil, errors.New("decryption unavailable or inside transaction")
	}
	return secrets.base.Decrypt(ctx, scope, encrypted)
}

type referenceExecutionFactory struct {
	*openaiadapter.Factory
	mode                          string
	now                           time.Time
	transactions                  *referenceExecutionTrackedTransactions
	submits, preflights           int
	borrowedSecrets               [][]byte
	beforePreflight, beforeSubmit func()
}

func (factory *referenceExecutionFactory) NewReferenceImageRuntime(config app.ProviderRuntimeConfig) (app.ReferenceImageRuntime, error) {
	factory.borrowedSecrets = append(factory.borrowedSecrets, config.Credentials)
	if len(config.Credentials) == 0 || factory.transactions.depth != 0 {
		return nil, errors.New("invalid runtime secret boundary")
	}
	return factory, nil
}

func (factory *referenceExecutionFactory) Preflight(_ context.Context, _ app.ReferenceImageSubmission) error {
	factory.preflights++
	if factory.transactions.depth != 0 || factory.mode == "preflight" {
		return errors.New("preflight rejected")
	}
	if factory.beforePreflight != nil {
		factory.beforePreflight()
	}
	return nil
}

func (factory *referenceExecutionFactory) Submit(_ context.Context, input app.ReferenceImageSubmission, dispatch domain.ReferenceCallDispatch) (app.ReferenceImageObservation, error) {
	factory.submits++
	if factory.transactions.depth != 0 {
		return app.ReferenceImageObservation{}, errors.New("Submit ran inside database transaction")
	}
	if factory.beforeSubmit != nil {
		factory.beforeSubmit()
	}
	if factory.mode == "not_sent" {
		return app.ReferenceImageObservation{}, errors.New("local Submit precondition failed")
	}
	result := app.ReferenceImageObservation{CallKey: input.Call.CallKey, SubmissionToken: dispatch.SubmissionToken, ObservedAt: factory.now, Status: app.ReferenceImageStaged}
	if factory.mode == "foreign_observation" {
		result.SubmissionToken = uuid.NewString()
	}
	if factory.mode == "invalid_disposition" {
		result.Status = "not_sent"
		result.ReasonCode = "submit_not_attempted"
		return result, nil
	}
	if factory.mode == "unknown" {
		result.Status, result.ReasonCode = app.ReferenceImageOutcomeUnknown, "transport_failed"
		return result, nil
	}
	if factory.mode == "rejected" {
		result.Status, result.ReasonCode = app.ReferenceImageOutputRejected, "invalid_png_contents"
		return result, nil
	}
	result.Output = &domain.ProviderOutput{OutputKey: "image", StagingObjectKey: "staging/reference/" + input.WorkspaceID + "/" + input.ProjectID + "/" + input.Call.ExecutionRef.ID + "/" + input.Call.CallKey + "/" + dispatch.SubmissionToken + "/image.png", SHA256: strings.Repeat("a", 64), MediaType: "image/png", Bytes: 100, Width: input.Slot.MinWidth, Height: input.Slot.MinHeight}
	result.Usage.ImageCount = 1
	return result, nil
}

// Real TLS/JSON/PNG transport, with a private-object boundary spy. No paid
// Provider or external storage is involved; PostgreSQL receipt writes are real.
func referenceExecutionHTTPFactory(t *testing.T, assertCommitted func()) (*openaiadapter.Factory, *referenceExecutionHTTPObjects) {
	t.Helper()
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		assertCommitted()
		var body struct {
			Size string `json:"size"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error("invalid HTTP body")
			w.WriteHeader(400)
			return
		}
		var width, height int
		if n, err := fmt.Sscanf(body.Size, "%dx%d", &width, &height); err != nil || n != 2 || width < 1 || height < 1 || width > 3840 || height > 3840 {
			t.Error("invalid frozen dimensions")
			w.WriteHeader(400)
			return
		}
		var output bytes.Buffer
		if err := png.Encode(&output, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"data":[{"b64_json":%q}]}`, base64.StdEncoding.EncodeToString(output.Bytes()))
	}))
	store := &referenceExecutionHTTPObjects{}
	t.Cleanup(func() {
		server.Close()
		if requests != 1 || store.writes != 1 {
			t.Errorf("HTTP/staging attempts=%d/%d", requests, store.writes)
		}
	})
	client := server.Client()
	transport := client.Transport
	client.Transport = referenceExecutionHTTPRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.openai.com" || r.URL.Path != "/v1/images/generations" {
			t.Error("wrong fixed endpoint")
		}
		r.URL.Host = server.Listener.Addr().String()
		return transport.RoundTrip(r)
	})
	return openaiadapter.NewFactory(client, store, time.Now), store
}

func assertReferenceStandaloneExecution(t *testing.T, ctx context.Context, database *generationtestgorm.Database, actor app.Actor, fixture referencePreparationFixture, configuration referenceExecutionFixture, userID string) {
	t.Helper()
	t.Logf("committed Reference test owner: user=%s workspace=%s project=%s", userID, fixture.execution.WorkspaceID, fixture.execution.ProjectID)
	fixturegorm.RegisterOwnedFixtureCleanup(t, database, fixturegorm.OwnedFixture{UserID: userID, WorkspaceID: fixture.execution.WorkspaceID, ProjectID: fixture.execution.ProjectID})
	store := generationgorm.New(database)
	var rows []model.GenerationReferenceProviderCall
	if err := database.Where("execution_id = ?", fixture.execution.ID).Order("bundle_index, slot_key").Find(&rows).Error; err != nil || len(rows) == 0 {
		t.Fatalf("read committed calls: %v", err)
	}
	command := app.ClaimReferenceCallCommand{WorkspaceID: fixture.execution.WorkspaceID, ProjectID: fixture.execution.ProjectID, ExecutionRef: domain.GenerationRevisionRef{ID: fixture.execution.ID, Revision: fixture.execution.Revision, ContentHash: fixture.execution.ContentHash}, CallKey: rows[0].CallKey, ExpectedRevision: 1}
	read := func(ctx context.Context) (domain.ReferenceCallState, error) {
		var state domain.ReferenceCallState
		err := store.WithinReferenceCallExecution(ctx, func(repo app.ReferenceCallDispatchRepository) error {
			var err error
			state, err = repo.FindReferenceCallState(ctx, command.WorkspaceID, command.ProjectID, command.ExecutionRef.ID, command.CallKey)
			return err
		})
		return state, err
	}
	factory, objects := referenceExecutionHTTPFactory(t, func() {
		// The HTTP handler is independent of the execution goroutine/connection.
		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		state, err := read(checkCtx)
		if err != nil || state.Status != domain.ProviderCallDispatching || state.Receipt != nil {
			t.Errorf("HTTP did not observe a committed dispatch: %+v %v", state, err)
		}
	})
	objects.beforeRead = func() error {
		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		var quarantined model.GenerationReferenceStagedMedia
		if err := database.WithContext(checkCtx).Where("call_key = ?", command.CallKey).First(&quarantined).Error; err != nil {
			return err
		}
		if quarantined.State != "quarantined" || quarantined.Revision != 1 {
			return errors.New("object read did not observe committed quarantine")
		}
		return nil
	}
	registry, err := app.NewMediaFactoryRegistry([]app.MediaAdapterFactory{factory})
	if err != nil {
		t.Fatal(err)
	}
	service, err := app.NewReferenceCallExecutionService(store, registry, configuration.secrets, time.Now, uuid.NewString)
	if err != nil {
		t.Fatal(err)
	}
	media, err := app.NewReferenceStagedMediaService(store, objects, domain.ReferenceObjectStoreRef{Profile: "minio", Bucket: "lanverse"}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if address := os.Getenv("LANVERSE_TEST_TEMPORAL_ADDRESS"); address != "" {
		recovery, err := app.NewReferenceCallDispatchService(store, registry, time.Now, uuid.NewString)
		if err != nil {
			t.Fatal(err)
		}
		executeReferenceCallThroughTemporal(t, ctx, database, address, actor, command, service, recovery, media, false, nil)
		// A different, already-fenced call simulates a Worker lost after Claim.
		// Backdate only the injected dispatch clock; production still freezes its
		// full 180-second timeout and recovery uses the real wall clock.
		if len(rows) < 2 {
			t.Fatal("missing second call for expiry recovery")
		}
		lost := command
		lost.CallKey = rows[1].CallKey
		executeReferenceCallThroughTemporal(t, ctx, database, address, actor, lost, service, recovery, media, true, func() {
			claimClock := time.Now().UTC().Add(-165 * time.Second)
			dispatch, err := app.NewReferenceCallDispatchService(store, registry, func() time.Time { return claimClock }, uuid.NewString)
			if err != nil {
				t.Fatal(err)
			}
			claimed, err := dispatch.Claim(ctx, actor, lost)
			if err != nil || !claimed.ShouldDispatch {
				t.Fatalf("seed lost dispatch: %v", err)
			}
		})
		unknown, err := service.Execute(ctx, actor, lost)
		if err != nil || unknown.Status != domain.ProviderCallOutcomeUnknown || unknown.Receipt != nil {
			t.Fatalf("lost dispatch was resent or published: %+v %v", unknown, err)
		}
	} else {
		t.Log("Temporal not configured: verifying direct committed execution only")
	}
	result, err := service.Execute(ctx, actor, command)
	if err != nil || result.Status != domain.ProviderCallSucceeded || result.Receipt == nil || result.Receipt.Output == nil {
		t.Fatalf("committed TLS execution: %+v %v", result, err)
	}
	persisted, err := read(ctx)
	if err != nil || !reflect.DeepEqual(persisted, result) {
		t.Fatalf("receipt not visible after commit: %v", err)
	}
	replay, err := service.Execute(ctx, actor, command)
	if err != nil || !reflect.DeepEqual(replay, result) {
		t.Fatalf("committed execution replay: %v", err)
	}
	mediaCommand := app.MaterializeReferenceStagedMediaCommand{WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID, ExecutionRef: command.ExecutionRef, CallKey: command.CallKey, ReceiptRef: domain.GenerationActionRef{ID: result.Receipt.SubmissionToken, ContentHash: result.Receipt.ContentHash}}
	staged, err := media.Materialize(ctx, actor, mediaCommand)
	if err != nil || staged.State != "ready_for_review" || staged.RightsObservation != "not_assessed" {
		t.Fatalf("committed media: %+v %v", staged, err)
	}
	repeated, err := media.Materialize(ctx, actor, mediaCommand)
	if err != nil || !reflect.DeepEqual(staged, repeated) {
		t.Fatalf("committed media replay: %v", err)
	}
	var stored model.GenerationReferenceStagedMedia
	if err := database.Where("id = ?", staged.ID).First(&stored).Error; err != nil || stored.ContentHash != staged.ContentHash {
		t.Fatalf("media commit not visible: %v", err)
	}
	var artifacts int64
	if err := database.Model(&model.Artifact{}).Where("project_id = ?", command.ProjectID).Count(&artifacts).Error; err != nil || artifacts != 0 {
		t.Fatalf("staging published an Artifact: %d %v", artifacts, err)
	}
}

type referenceExecutionHTTPRoundTrip func(*http.Request) (*http.Response, error)

func (f referenceExecutionHTTPRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

type referenceExecutionHTTPObjects struct {
	mu         sync.Mutex
	writes     int
	contents   []byte
	key        string
	beforeRead func() error
}

func (store *referenceExecutionHTTPObjects) EnsurePrivateObject(ctx context.Context, key string, contents []byte, mime, hash string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.writes++
	if ctx.Err() != nil || !strings.HasPrefix(key, "staging/reference/") || mime != "image/png" || len(hash) != 64 || len(contents) == 0 {
		return errors.New("invalid staged object")
	}
	store.key, store.contents = key, bytes.Clone(contents)
	return nil
}

func (store *referenceExecutionHTTPObjects) ReadVerified(ctx context.Context, key string, size int64, hash string, max int64) ([]byte, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	digest := sha256.Sum256(store.contents)
	if store.beforeRead != nil {
		if err := store.beforeRead(); err != nil {
			return nil, err
		}
	}
	if ctx.Err() != nil || key != store.key || size != int64(len(store.contents)) || size > max || hash != hex.EncodeToString(digest[:]) {
		return nil, errors.New("invalid private object read")
	}
	return bytes.Clone(store.contents), nil
}

package workflow_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	openaiadapter "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/openai"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/google/uuid"
)

type referencePreparationFixture struct {
	service   *generationapp.ReferenceExecutionPreparationService
	command   generationapp.PrepareInitialReferenceExecutionCommand
	execution domain.ReferenceExecution
}

func assertInitialReferenceExecutionPreparation(t *testing.T, ctx context.Context, transactions generationapp.ReferenceExecutionTransactions, actor generationapp.Actor, target generationapp.ReferenceGenerationTarget, authorization domain.ReferenceExecutionAuthorization, now time.Time) referencePreparationFixture {
	t.Helper()
	registry, err := generationapp.NewMediaFactoryRegistry([]generationapp.MediaAdapterFactory{openaiadapter.NewFactory(nil, nil, nil)})
	if err != nil {
		t.Fatal(err)
	}
	clock := func() time.Time { return now }
	command := generationapp.PrepareInitialReferenceExecutionCommand{WorkspaceID: target.WorkspaceID, ProjectID: target.ProjectID, TargetRef: domain.GenerationRevisionRef{ID: target.ID, Revision: target.Revision, ContentHash: target.ContentHash}, AuthorizationRef: domain.GenerationActionRef{ID: authorization.HumanActionRef, ContentHash: authorization.ContentHash}, IdempotencyKey: "reference-prepare-initial"}
	for _, fault := range []string{"missing_compiler", "missing_slot", "wrong_slot", "request_hash", "manifest_hash", "compiler_hash"} {
		var factory generationapp.MediaAdapterFactory = &referenceCompilationFaultFactory{Factory: openaiadapter.NewFactory(nil, nil, nil), fault: fault}
		if fault == "missing_compiler" {
			factory = referenceAuthorizationMediaFactory{}
		}
		faultRegistry, err := generationapp.NewMediaFactoryRegistry([]generationapp.MediaAdapterFactory{factory})
		if err != nil {
			t.Fatal(err)
		}
		idCalls := 0
		failed, err := generationapp.NewReferenceExecutionPreparationService(transactions, faultRegistry, clock, func() string { idCalls++; return uuid.NewString() })
		if err != nil {
			t.Fatal(err)
		}
		if value, err := failed.PrepareInitial(ctx, actor, command); err == nil || idCalls != 0 || !reflect.DeepEqual(value, domain.ReferenceExecution{}) {
			t.Fatalf("compiler fault %s did not fail before writes: %v", fault, err)
		}
	}
	// Fail after snapshot/head insertion, before a valid command receipt exists.
	for _, receiptID := range []string{"invalid-receipt-id", authorization.HumanActionRef} {
		calls := 0
		service, err := generationapp.NewReferenceExecutionPreparationService(transactions, registry, clock, func() string {
			calls++
			if calls == 1 {
				return uuid.NewString()
			}
			return receiptID
		})
		if err != nil {
			t.Fatal(err)
		}
		got, err := service.PrepareInitial(ctx, actor, command)
		if err == nil || calls != 2 || !reflect.DeepEqual(got, domain.ReferenceExecution{}) {
			t.Fatalf("receipt failure did not roll back preparation: calls=%d err=%v", calls, err)
		}
	}
	// Simulate a storage failure after the real Job and full call set are written.
	// The outer transaction must also remove the already published Snapshot/Head.
	failing, err := generationapp.NewReferenceExecutionPreparationService(referenceCallWriteFailureTransactions{transactions}, registry, clock, uuid.NewString)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := failing.PrepareInitial(ctx, actor, command); err == nil || !reflect.DeepEqual(got, domain.ReferenceExecution{}) {
		t.Fatal("call persistence failure did not roll back execution preparation")
	}
	service, err := generationapp.NewReferenceExecutionPreparationService(transactions, registry, clock, uuid.NewString)
	if err != nil {
		t.Fatal(err)
	}
	value, err := service.PrepareInitial(ctx, actor, command)
	if err != nil {
		t.Fatal(err)
	}
	if value.ReadSet.TargetRef != command.TargetRef || value.ReadSet.AuthorizationRef != command.AuthorizationRef || value.ReadSet.BindingRef != authorization.SelectedProjectProviderBindingVersionRef {
		t.Fatal("execution lost exact input")
	}
	replayed, err := service.PrepareInitial(ctx, actor, command)
	if err != nil || !reflect.DeepEqual(replayed, value) {
		t.Fatalf("preparation replay: %v", err)
	}
	newKey := command
	newKey.IdempotencyKey = "another-preparation"
	if got, err := service.PrepareInitial(ctx, actor, newKey); err == nil || !reflect.DeepEqual(got, domain.ReferenceExecution{}) {
		t.Fatal("new key bypassed initial Head")
	}
	bad := command
	bad.AuthorizationRef.ContentHash = strings.Repeat("f", 64)
	if _, err := service.PrepareInitial(ctx, actor, bad); err == nil {
		t.Fatal("wrong authorization hash accepted")
	}
	bad = command
	bad.ExpectedHeadRevision = 1
	if _, err := service.PrepareInitial(ctx, actor, bad); err == nil {
		t.Fatal("noninitial preparation accepted")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if decoded, err := domain.DecodeReferenceExecution(raw); err != nil || !reflect.DeepEqual(decoded, value) {
		t.Fatalf("persisted snapshot: %v", err)
	}
	for _, forbidden := range []string{"prompt", "api_key", "ciphertext", "https://", "base64"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatal("execution leaked invocation material")
		}
	}
	return referencePreparationFixture{service, command, value}
}

type referenceCallWriteFailureTransactions struct {
	generationapp.ReferenceExecutionTransactions
}

func (tx referenceCallWriteFailureTransactions) WithinReferenceExecution(ctx context.Context, operation func(generationapp.ReferenceExecutionRepository) error) error {
	return tx.ReferenceExecutionTransactions.WithinReferenceExecution(ctx, func(repo generationapp.ReferenceExecutionRepository) error {
		return operation(referenceCallWriteFailureRepository{repo})
	})
}

type referenceCallWriteFailureRepository struct {
	generationapp.ReferenceExecutionRepository
}

func (repo referenceCallWriteFailureRepository) PublishReferenceProviderJob(ctx context.Context, workspace, project string, job domain.ReferenceProviderJob, calls []domain.ReferenceProviderCall) error {
	if err := repo.ReferenceExecutionRepository.PublishReferenceProviderJob(ctx, workspace, project, job, calls); err != nil {
		return err
	}
	return errors.New("injected Reference Provider call persistence failure")
}

type referenceCompilationFaultFactory struct {
	*openaiadapter.Factory
	fault string
}

func (factory *referenceCompilationFaultFactory) CompileReferenceImages(target generationapp.ReferenceGenerationTarget, brief agentapp.AcceptedReferenceBrief, profile domain.ProviderModelProfileVersion) (generationapp.ReferenceImageCompilation, error) {
	value, err := factory.Factory.CompileReferenceImages(target, brief, profile)
	if err != nil {
		return value, err
	}
	switch factory.fault {
	case "missing_slot":
		value.Requests = value.Requests[:len(value.Requests)-1]
	case "wrong_slot":
		value.Requests[0].SlotKey = "unrelated"
	case "request_hash":
		value.Requests[0].ContentHash = strings.Repeat("f", 64)
	case "manifest_hash":
		value.ManifestHash = strings.Repeat("f", 64)
	case "compiler_hash":
		value.Descriptor.CompilerRef.ContentHash = strings.Repeat("f", 64)
	}
	return value, nil
}

func assertReferencePreparationRejected(t *testing.T, ctx context.Context, actor generationapp.Actor, fixture referencePreparationFixture, newKey bool) {
	t.Helper()
	command := fixture.command
	if newKey {
		command.IdempotencyKey = "rejected-preparation"
	}
	value, err := fixture.service.PrepareInitial(ctx, actor, command)
	if err == nil || !reflect.DeepEqual(value, domain.ReferenceExecution{}) {
		t.Fatal("stale preparation returned an execution")
	}
}

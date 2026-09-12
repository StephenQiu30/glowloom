package application

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstore"
)

var ErrReferenceStagedMediaNotFound = errors.New("Reference staged media was not found")
var ErrReferenceStagedMediaConflict = errors.New("Reference staged media state conflicts with expected revision")

type ReferenceStagedMediaRepository interface {
	ReferenceCallDispatchRepository
	FindReferenceStagedMedia(context.Context, string, string, string) (domain.ReferenceStagedMedia, error)
	InsertReferenceStagedMedia(context.Context, domain.ReferenceStagedMedia) error
	UpdateReferenceStagedMedia(context.Context, domain.ReferenceStagedMedia, domain.ReferenceStagedMedia) error
}

type ReferenceStagedMediaTransactions interface {
	WithinReferenceStagedMedia(context.Context, func(ReferenceStagedMediaRepository) error) error
}

type ReferenceStagedObjectReader interface {
	ReadVerified(context.Context, string, int64, string, int64) ([]byte, error)
}

type MaterializeReferenceStagedMediaCommand struct {
	WorkspaceID  string                       `json:"workspace_id"`
	ProjectID    string                       `json:"project_id"`
	ExecutionRef domain.GenerationRevisionRef `json:"execution_ref"`
	CallKey      string                       `json:"call_key"`
	ReceiptRef   domain.GenerationActionRef   `json:"receipt_ref"`
}

type ReferenceStagedMediaService struct {
	transactions ReferenceStagedMediaTransactions
	objects      ReferenceStagedObjectReader
	location     domain.ReferenceObjectStoreRef
	now          func() time.Time
}

func NewReferenceStagedMediaService(transactions ReferenceStagedMediaTransactions, objects ReferenceStagedObjectReader, location domain.ReferenceObjectStoreRef, now func() time.Time) (*ReferenceStagedMediaService, error) {
	if transactions == nil || objects == nil || now == nil || !location.Valid() || location.ObjectKey != "" {
		return nil, errors.New("Reference staged media dependencies are required")
	}
	return &ReferenceStagedMediaService{transactions, objects, location, now}, nil
}

func (service *ReferenceStagedMediaService) Materialize(ctx context.Context, actor Actor, command MaterializeReferenceStagedMediaCommand) (domain.ReferenceStagedMedia, error) {
	if !validReferenceCallScope(actor, command.WorkspaceID, command.ProjectID, command.ExecutionRef, command.CallKey) || !command.ReceiptRef.Valid() {
		return domain.ReferenceStagedMedia{}, invalid("Invalid Reference staged media command")
	}
	var observed domain.ReferenceStagedMedia
	err := service.transactions.WithinReferenceStagedMedia(ctx, func(repo ReferenceStagedMediaRepository) error {
		expected, err := service.expected(ctx, repo, actor, command)
		if err != nil {
			return err
		}
		observed, err = repo.FindReferenceStagedMedia(ctx, command.WorkspaceID, command.ProjectID, command.CallKey)
		if errors.Is(err, ErrReferenceStagedMediaNotFound) {
			if err := repo.InsertReferenceStagedMedia(ctx, expected); err != nil {
				return err
			}
			observed = expected
			return nil
		}
		if err != nil {
			return err
		}
		initial, err := domain.InitialReferenceStagedMedia(observed)
		if err != nil || !reflect.DeepEqual(initial, expected) {
			return conflict("Reference staged media differs from successful receipt")
		}
		return nil
	})
	if err != nil {
		return domain.ReferenceStagedMedia{}, err
	}
	if observed.State != "quarantined" {
		return observed, nil
	}

	// The quarantine registration has physically committed before object IO.
	contents, readErr := service.objects.ReadVerified(ctx, observed.ObjectStoreRef.ObjectKey, observed.ByteSize, observed.SHA256, observed.ByteSize)
	defer clear(contents)
	if err := ctx.Err(); err != nil {
		return domain.ReferenceStagedMedia{}, err
	}
	var desired domain.ReferenceStagedMedia
	if readErr == nil {
		desired, err = domain.CompleteReferenceStagedMedia(observed, contents, service.now())
	} else {
		reason := ""
		switch {
		case errors.Is(readErr, objectstore.ErrObjectChecksumMismatch):
			reason = "checksum_mismatch"
		case errors.Is(readErr, objectstore.ErrObjectSizeMismatch):
			reason = "size_mismatch"
		default:
			return domain.ReferenceStagedMedia{}, &Error{Code: "staged_object_unavailable", Message: "Reference staged object storage is unavailable", Status: 503}
		}
		desired, err = domain.RejectReferenceStagedMedia(observed, reason, service.now())
	}
	if err != nil {
		return domain.ReferenceStagedMedia{}, err
	}
	if err := ctx.Err(); err != nil {
		return domain.ReferenceStagedMedia{}, err
	}
	var result domain.ReferenceStagedMedia
	err = service.transactions.WithinReferenceStagedMedia(ctx, func(repo ReferenceStagedMediaRepository) error {
		expected, err := service.expected(ctx, repo, actor, command)
		if err != nil {
			return err
		}
		current, err := repo.FindReferenceStagedMedia(ctx, command.WorkspaceID, command.ProjectID, command.CallKey)
		if err != nil {
			return err
		}
		initial, err := domain.InitialReferenceStagedMedia(current)
		if err != nil || !reflect.DeepEqual(initial, expected) || !reflect.DeepEqual(initial, observed) {
			return conflict("Reference staged media changed during validation")
		}
		if current.State != "quarantined" {
			// Concurrent validation may finish first. Do not overwrite an opposite
			// observation or its immutable validation timestamp.
			if current.State != desired.State || current.FailureCode != desired.FailureCode {
				return conflict("Reference staged validation outcomes conflict")
			}
			result = current
			return nil
		}
		if err := repo.UpdateReferenceStagedMedia(ctx, current, desired); err != nil {
			return err
		}
		result = desired
		return nil
	})
	if err != nil {
		return domain.ReferenceStagedMedia{}, err
	}
	return result, nil
}

func (service *ReferenceStagedMediaService) expected(ctx context.Context, repo ReferenceStagedMediaRepository, actor Actor, command MaterializeReferenceStagedMediaCommand) (domain.ReferenceStagedMedia, error) {
	execution, state, err := readReferenceCall(ctx, repo, actor, command.WorkspaceID, command.ProjectID, command.ExecutionRef, command.CallKey)
	if err != nil {
		return domain.ReferenceStagedMedia{}, err
	}
	if state.Status != domain.ProviderCallSucceeded || state.Receipt == nil || state.Receipt.WorkspaceID != command.WorkspaceID || state.Receipt.ProjectID != command.ProjectID || state.Receipt.Call.ExecutionRef != command.ExecutionRef || state.Receipt.Call.CallKey != command.CallKey || state.Receipt.SubmissionToken != command.ReceiptRef.ID || state.Receipt.ContentHash != command.ReceiptRef.ContentHash {
		return domain.ReferenceStagedMedia{}, conflict("Reference staged media requires the exact successful receipt")
	}
	target, err := repo.FindReferenceGenerationTarget(ctx, command.WorkspaceID, command.ProjectID, execution.ReadSet.TargetRef.ID)
	if err != nil {
		return domain.ReferenceStagedMedia{}, err
	}
	if referenceGenerationTargetRef(target) != execution.ReadSet.TargetRef || !slices.ContainsFunc(target.OutputContract.Slots, func(slot domain.ReferenceOutputSlot) bool { return reflect.DeepEqual(slot, state.Receipt.Slot) }) {
		return domain.ReferenceStagedMedia{}, conflict("Reference staged media differs from frozen Target")
	}
	location := service.location
	if state.Receipt.Output == nil {
		return domain.ReferenceStagedMedia{}, conflict("Reference receipt has no output")
	}
	location.ObjectKey = state.Receipt.Output.StagingObjectKey
	return domain.NewReferenceStagedMedia(*state.Receipt, location)
}

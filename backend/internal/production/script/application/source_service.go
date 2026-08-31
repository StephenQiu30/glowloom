package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	"github.com/StephenQiu30/lanverse/backend/internal/production/script/domain"
)

const acceptSourceOperation = "script_source.accept"

type AcceptSourceCommand struct {
	ProjectID, DocumentRevisionID string
	ExpectedHeadRevision          int64
	ExpectedHeadHash              *string
	IdempotencyKey                string
}

type SourceConfig struct {
	Now   func() time.Time
	NewID func() string
}

type SourceTransactions interface {
	WithinSourceTransaction(context.Context, func(SourceRepository) error) error
}

type SourceRepository interface {
	ProjectWorkspace(context.Context, Actor, string, bool) (string, error)
	GetAnalysis(context.Context, string) (domain.Analysis, error)
	FindReceipt(context.Context, string, string, string) (platformcommand.Receipt, error)
	CreateReceipt(context.Context, platformcommand.Receipt) error
	LockSourceHead(context.Context, string) (domain.SourceHead, error)
	FindSpanIndex(context.Context, string) (domain.SourceSpanIndex, error)
	CreateSpanIndex(context.Context, domain.SourceSpanIndex) error
	CreateSourceHead(context.Context, domain.SourceHead, time.Time) error
	AdvanceSourceHead(context.Context, domain.SourceHead, int64, string, time.Time) error
	CreateSourceCollectionReceipt(context.Context, domain.SourceCollectionReceipt) error
	GetAcceptedSource(context.Context, string, string) (domain.AcceptedSource, error)
}

type SourceService struct {
	transactions SourceTransactions
	config       SourceConfig
}

func NewSourceService(transactions SourceTransactions, config SourceConfig) *SourceService {
	if config.Now == nil {
		config.Now = func() time.Time { return time.Now().UTC() }
	}
	if config.NewID == nil {
		config.NewID = func() string { return "" }
	}
	return &SourceService{transactions: transactions, config: config}
}

func (service *SourceService) Accept(ctx context.Context, actor Actor, command AcceptSourceCommand) (domain.AcceptedSource, error) {
	if service == nil || service.transactions == nil || strings.TrimSpace(command.ProjectID) == "" ||
		strings.TrimSpace(command.DocumentRevisionID) == "" || strings.TrimSpace(command.IdempotencyKey) == "" ||
		command.ExpectedHeadRevision < 0 ||
		(command.ExpectedHeadRevision == 0) != (command.ExpectedHeadHash == nil) ||
		(command.ExpectedHeadHash != nil && len(*command.ExpectedHeadHash) != 64) {
		return domain.AcceptedSource{}, invalid("Invalid Script Source acceptance command")
	}
	inputHash, err := platformcommand.InputHash(command)
	if err != nil {
		return domain.AcceptedSource{}, err
	}
	var accepted domain.AcceptedSource
	err = service.transactions.WithinSourceTransaction(ctx, func(repo SourceRepository) error {
		workspaceID, authorizeErr := repo.ProjectWorkspace(ctx, actor, command.ProjectID, true)
		if authorizeErr != nil {
			return authorizeErr
		}
		receipt, receiptErr := repo.FindReceipt(ctx, workspaceID, acceptSourceOperation, command.IdempotencyKey)
		if receiptErr == nil {
			var replay struct {
				Accepted domain.AcceptedSource `json:"accepted"`
			}
			if receipt.InputHash != inputHash {
				return sourceError("idempotency_conflict", "Idempotency key was used with different input", 409)
			}
			if json.Unmarshal(receipt.Result, &replay) != nil {
				return errors.New("Script Source command receipt is invalid")
			}
			accepted = replay.Accepted
			return nil
		}
		if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		analysis, analysisErr := repo.GetAnalysis(ctx, command.DocumentRevisionID)
		if analysisErr != nil {
			return analysisErr
		}
		if analysis.Document.ProjectID != command.ProjectID || analysis.Document.WorkspaceID != workspaceID ||
			analysis.Revision.ID != command.DocumentRevisionID || analysis.Revision.DocumentID != analysis.Document.ID ||
			!utf8.ValidString(analysis.Revision.RawText) || !utf8.ValidString(analysis.Revision.NormalizedText) ||
			analysis.Revision.NormalizedText == "" || strings.Contains(analysis.Revision.NormalizedText, "\r") ||
			analysis.Revision.NormalizedHash != textHash(analysis.Revision.NormalizedText) ||
			analysis.Revision.CodepointCount != utf8.RuneCountInString(analysis.Revision.NormalizedText) {
			return sourceError("source_hash_drift", "Script Source bytes or code-point index drifted", 409)
		}
		head, headErr := repo.LockSourceHead(ctx, command.ProjectID)
		if headErr != nil && !errors.Is(headErr, ErrNotFound) {
			return headErr
		}
		if command.ExpectedHeadRevision == 0 {
			if head.Exists {
				return sourceError("head_conflict", "Script Source Head already exists", 409)
			}
		} else if !head.Exists || head.HeadRevision != command.ExpectedHeadRevision ||
			head.HeadHash != *command.ExpectedHeadHash {
			return sourceError("head_conflict", "Script Source Head changed", 409)
		}

		now := service.config.Now().UTC()
		index, indexErr := repo.FindSpanIndex(ctx, analysis.Revision.ID)
		if errors.Is(indexErr, ErrNotFound) {
			manifest, marshalErr := json.Marshal(map[string]any{
				"contract_id": "source-span-index-production", "document_revision_id": analysis.Revision.ID,
				"source_hash":          analysis.Revision.NormalizedHash,
				"codepoint_index_rule": "unicode-code-point", "ranges": []map[string]any{{"start": 0, "end": analysis.Revision.CodepointCount}},
			})
			if marshalErr != nil {
				return marshalErr
			}
			indexHash, hashErr := platformcanonical.Hash(manifest)
			if hashErr != nil {
				return hashErr
			}
			index = domain.SourceSpanIndex{
				ID: service.config.NewID(), WorkspaceID: workspaceID, ProjectID: command.ProjectID,
				DocumentRevisionID: analysis.Revision.ID, SourceHash: analysis.Revision.NormalizedHash,
				NewlineNormalization: "lf", CodepointIndexRule: "unicode-code-point",
				CodepointCount: analysis.Revision.CodepointCount, UTF8ByteCount: len([]byte(analysis.Revision.NormalizedText)),
				IndexManifest: manifest, ContentHash: indexHash, CreatedBy: actor.UserID, CreatedAt: now,
			}
			if createErr := repo.CreateSpanIndex(ctx, index); createErr != nil {
				return createErr
			}
		} else if indexErr != nil {
			return indexErr
		}
		identity := domain.SourceVersionIdentity{
			OwnerKind: "production/script", LogicalID: analysis.Document.ID, VersionID: analysis.Revision.ID,
			Revision: int64(analysis.Revision.VersionNo), ContentHash: analysis.Revision.NormalizedHash,
			CreatedAt: analysis.Revision.CreatedAt.UTC(),
		}
		members, marshalErr := json.Marshal([]any{identity, map[string]any{
			"owner_kind": "production/script", "logical_id": analysis.Document.ID + ":span-index",
			"version_id": index.ID, "revision": int64(analysis.Revision.VersionNo), "content_hash": index.ContentHash,
			"created_at": index.CreatedAt,
		}})
		if marshalErr != nil {
			return marshalErr
		}
		membersHash, hashErr := platformcanonical.Hash(members)
		if hashErr != nil {
			return hashErr
		}
		newHeadRevision := head.HeadRevision + 1
		headMaterial, _ := json.Marshal(map[string]any{
			"contract_id": "script-source-head-production", "project_id": command.ProjectID,
			"document_revision_id": analysis.Revision.ID, "span_index_id": index.ID,
			"head_revision": newHeadRevision, "members_hash": membersHash,
		})
		headHash, hashErr := platformcanonical.Hash(headMaterial)
		if hashErr != nil {
			return hashErr
		}
		collectionMaterial, _ := json.Marshal(map[string]any{
			"contract_id": "script-source-collection-production", "workspace_id": workspaceID,
			"project_id": command.ProjectID, "head_revision": newHeadRevision, "head_hash": headHash,
			"members": json.RawMessage(members), "members_hash": membersHash,
		})
		collectionRootHash, hashErr := platformcanonical.Hash(collectionMaterial)
		if hashErr != nil {
			return hashErr
		}
		newHead := domain.SourceHead{
			Exists: true, ProjectID: command.ProjectID, WorkspaceID: workspaceID,
			DocumentLogicalID: analysis.Document.ID, DocumentRevisionID: analysis.Revision.ID,
			SpanIndexID: index.ID, HeadRevision: newHeadRevision, HeadHash: headHash,
		}
		if head.Exists {
			if advanceErr := repo.AdvanceSourceHead(ctx, newHead, head.HeadRevision, head.HeadHash, now); advanceErr != nil {
				return advanceErr
			}
		} else if createErr := repo.CreateSourceHead(ctx, newHead, now); createErr != nil {
			return createErr
		}
		collectionReceiptID := service.config.NewID()
		receiptMaterial, _ := json.Marshal(map[string]any{
			"collection_receipt_id": collectionReceiptID, "collection_root_hash": collectionRootHash,
			"source_acceptance_ref": command.IdempotencyKey,
		})
		receiptHash, hashErr := platformcanonical.Hash(receiptMaterial)
		if hashErr != nil {
			return hashErr
		}
		if createErr := repo.CreateSourceCollectionReceipt(ctx, domain.SourceCollectionReceipt{
			ID: collectionReceiptID, WorkspaceID: workspaceID, ProjectID: command.ProjectID,
			DocumentRevisionID: analysis.Revision.ID, SpanIndexID: index.ID, HeadRevision: newHeadRevision,
			HeadHash: headHash, Members: members, MembersHash: membersHash, CollectionRootHash: collectionRootHash,
			SourceAcceptanceRef: command.IdempotencyKey, ReceiptContentHash: receiptHash,
			CreatedBy: actor.UserID, CreatedAt: now,
		}); createErr != nil {
			return createErr
		}
		commandReceiptID := service.config.NewID()
		accepted = domain.AcceptedSource{
			Identity: identity, SpanIndexID: index.ID, SpanIndexHash: index.ContentHash,
			CodepointCount: index.CodepointCount, UTF8ByteCount: index.UTF8ByteCount,
			NewlineNormalization: index.NewlineNormalization, CodepointIndexRule: index.CodepointIndexRule,
			HeadRevision: newHeadRevision, HeadHash: headHash, CollectionRootHash: collectionRootHash,
			CollectionReceiptID: collectionReceiptID, CommandReceiptID: commandReceiptID,
		}
		result, marshalErr := json.Marshal(map[string]any{"accepted": accepted})
		if marshalErr != nil {
			return marshalErr
		}
		return repo.CreateReceipt(ctx, platformcommand.Receipt{
			ID: commandReceiptID, WorkspaceID: workspaceID, Operation: acceptSourceOperation,
			IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: analysis.Revision.ID,
			Result: result, CreatedBy: actor.UserID, CreatedAt: now,
		})
	})
	return accepted, err
}

func (service *SourceService) GetExact(ctx context.Context, actor Actor, projectID, revisionID string) (domain.AcceptedSource, error) {
	var accepted domain.AcceptedSource
	err := service.transactions.WithinSourceTransaction(ctx, func(repo SourceRepository) error {
		if _, authorizeErr := repo.ProjectWorkspace(ctx, actor, projectID, false); authorizeErr != nil {
			return authorizeErr
		}
		var queryErr error
		accepted, queryErr = repo.GetAcceptedSource(ctx, projectID, revisionID)
		return queryErr
	})
	return accepted, err
}

func ErrorCode(err error) string {
	var typed *Error
	if errors.As(err, &typed) {
		return typed.Code
	}
	return ""
}

func sourceError(code, message string, status int) error {
	return &Error{Code: code, Message: message, Status: status}
}

func textHash(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

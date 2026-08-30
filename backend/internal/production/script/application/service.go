package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	"github.com/StephenQiu30/lanverse/backend/internal/production/script/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/production/script/parser"
)

const (
	normalizerVersion = "unicode-codepoint-normalizer"
	analyzerVersion   = "screenplay-structure-analyzer"
	importOperation   = "script.import"
)

var ErrNotFound = errors.New("script resource not found")

type Error struct {
	Code, Message, NextAction string
	Status                    int
	Details                   map[string]any
}

func (value *Error) Error() string { return value.Message }

type Actor struct {
	UserID       string
	TokenVersion int
}

type MediaContent struct {
	WorkspaceID, MIMEType string
}

type MediaReader interface {
	Read(context.Context, Actor, string) (MediaContent, []byte, error)
}

type Repository interface {
	ProjectWorkspace(context.Context, Actor, string, bool) (string, error)
	FindReceipt(context.Context, string, string, string) (platformcommand.Receipt, error)
	CreateReceipt(context.Context, platformcommand.Receipt) error
	CreateAnalysis(context.Context, domain.Analysis) error
	GetAnalysis(context.Context, string) (domain.Analysis, error)
	GetCurrentAnalysis(context.Context, string) (domain.Analysis, error)
	ListDocuments(context.Context, string, int, int) ([]domain.Document, int, error)
}

type TransactionManager interface {
	WithinTransaction(context.Context, func(Repository) error) error
}

type Config struct {
	Now   func() time.Time
	NewID func() string
}

type Service struct {
	transactions TransactionManager
	media        MediaReader
	config       Config
}

type Preview struct {
	MediaVersionID, RawText, RawHash string
	CodepointCount                   int
}

type ImportCommand struct {
	ProjectID, InputType, Title, Language, RightsDeclaration, IdempotencyKey string
	Text, MediaVersionID                                                     *string
}

type importReceipt struct {
	RevisionID string `json:"revision_id"`
}

func NewService(transactions TransactionManager, media MediaReader, config Config) *Service {
	return &Service{transactions: transactions, media: media, config: config}
}

func (service *Service) Preview(ctx context.Context, actor Actor, projectID, mediaVersionID string) (Preview, error) {
	workspaceID, err := service.projectWorkspace(ctx, actor, projectID, true)
	if err != nil {
		return Preview{}, err
	}
	media, contents, err := service.media.Read(ctx, actor, mediaVersionID)
	if err != nil {
		return Preview{}, err
	}
	if media.WorkspaceID != workspaceID {
		return Preview{}, notFound("Media version not found")
	}
	rawText, err := parser.DecodeDocument(media.MIMEType, contents)
	if err != nil {
		return Preview{}, invalid("Document content could not be read")
	}
	return Preview{MediaVersionID: mediaVersionID, RawText: rawText, RawHash: hashText(rawText), CodepointCount: utf8.RuneCountInString(rawText)}, nil
}

func (service *Service) Import(ctx context.Context, actor Actor, command ImportCommand) (domain.Analysis, error) {
	command.Title = strings.TrimSpace(command.Title)
	command.Language = strings.TrimSpace(command.Language)
	command.RightsDeclaration = strings.TrimSpace(command.RightsDeclaration)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if command.ProjectID == "" || command.Title == "" || len([]rune(command.Title)) > 120 || command.Language == "" || len(command.Language) > 35 || command.RightsDeclaration == "" || len([]rune(command.RightsDeclaration)) > 1000 || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return domain.Analysis{}, invalid("Invalid script import request")
	}
	workspaceID, err := service.projectWorkspace(ctx, actor, command.ProjectID, true)
	if err != nil {
		return domain.Analysis{}, err
	}

	var rawText string
	var sourceMediaVersionID *string
	switch command.InputType {
	case "text":
		if command.Text == nil || command.MediaVersionID != nil || !utf8.ValidString(*command.Text) {
			return domain.Analysis{}, invalid("Text import requires UTF-8 text and no media version")
		}
		rawText = *command.Text
	case "media":
		if command.MediaVersionID == nil || command.Text != nil {
			return domain.Analysis{}, invalid("Media import requires one media version and no text")
		}
		media, contents, mediaErr := service.media.Read(ctx, actor, *command.MediaVersionID)
		if mediaErr != nil {
			return domain.Analysis{}, mediaErr
		}
		if media.WorkspaceID != workspaceID {
			return domain.Analysis{}, notFound("Media version not found")
		}
		rawText, mediaErr = parser.DecodeDocument(media.MIMEType, contents)
		if mediaErr != nil {
			return domain.Analysis{}, invalid("Document content could not be read")
		}
		value := *command.MediaVersionID
		sourceMediaVersionID = &value
	default:
		return domain.Analysis{}, invalid("Unsupported script import input type")
	}
	if strings.TrimSpace(rawText) == "" {
		return domain.Analysis{}, invalid("Script document must not be empty")
	}

	rawHash := hashText(rawText)
	inputHash, err := platformcommand.InputHash(struct {
		ProjectID, InputType, Title, Language, RightsDeclaration, RawHash string
		MediaVersionID                                                    *string
	}{command.ProjectID, command.InputType, command.Title, command.Language, command.RightsDeclaration, rawHash, sourceMediaVersionID})
	if err != nil {
		return domain.Analysis{}, err
	}

	parsed := parser.Analyze(rawText)
	now := service.config.Now().UTC()
	documentID := service.config.NewID()
	revisionID := service.config.NewID()
	analysis := domain.Analysis{
		Document: domain.Document{ID: documentID, WorkspaceID: workspaceID, ProjectID: command.ProjectID, Title: command.Title, SourceType: command.InputType, SourceMediaVersionID: sourceMediaVersionID, Language: command.Language, RightsDeclaration: command.RightsDeclaration, Status: "active", Revision: 1, CreatedBy: actor.UserID, CreatedAt: now},
		Revision: domain.Revision{ID: revisionID, WorkspaceID: workspaceID, DocumentID: documentID, VersionNo: 1, SourceType: command.InputType, SourceMediaVersionID: sourceMediaVersionID, RawText: rawText, RawHash: rawHash, NormalizedText: parsed.NormalizedText, NormalizedHash: hashText(parsed.NormalizedText), NormalizerVersion: normalizerVersion, NormalizationMap: parsed.NormalizationMap, CodepointCount: utf8.RuneCountInString(parsed.NormalizedText), AnalysisStatus: parsed.Status, AnalyzerVersion: analyzerVersion, CreatedBy: actor.UserID, CreatedAt: now},
	}
	for _, block := range parsed.Blocks {
		analysis.Revision.Blocks = append(analysis.Revision.Blocks, domain.Block{ID: service.config.NewID(), DocumentRevisionID: revisionID, Position: block.Position, Kind: block.Kind, SourceStart: block.SourceStart, SourceEnd: block.SourceEnd, TextHash: hashText(block.TextHashInput), Metadata: block.Metadata})
	}
	for _, issue := range parsed.Issues {
		analysis.Revision.Issues = append(analysis.Revision.Issues, domain.Issue{ID: service.config.NewID(), DocumentRevisionID: revisionID, Position: issue.Position, Code: issue.Code, Severity: issue.Severity, SourceStart: issue.SourceStart, SourceEnd: issue.SourceEnd, LineNumber: issue.LineNumber, ColumnNumber: issue.ColumnNumber, NextAction: issue.NextAction, Details: issue.Details})
	}

	err = service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		currentWorkspaceID, authorizeErr := repo.ProjectWorkspace(ctx, actor, command.ProjectID, true)
		if authorizeErr != nil {
			return authorizeErr
		}
		if currentWorkspaceID != workspaceID {
			return notFound("Project not found")
		}
		receipt, receiptErr := repo.FindReceipt(ctx, workspaceID, importOperation, command.IdempotencyKey)
		if receiptErr == nil {
			replayed, replayErr := platformcommand.Replay[importReceipt](receipt, inputHash)
			if errors.Is(replayErr, platformcommand.ErrInputMismatch) {
				return &Error{Code: "resource_conflict", Message: "Idempotency key was already used with different input", Status: 409}
			}
			if replayErr != nil {
				return replayErr
			}
			analysis, replayErr = repo.GetAnalysis(ctx, replayed.RevisionID)
			return replayErr
		}
		if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		if createErr := repo.CreateAnalysis(ctx, analysis); createErr != nil {
			return createErr
		}
		result, resultErr := platformcommand.Result(importReceipt{RevisionID: revisionID})
		if resultErr != nil {
			return resultErr
		}
		return repo.CreateReceipt(ctx, platformcommand.Receipt{ID: service.config.NewID(), WorkspaceID: workspaceID, Operation: importOperation, IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: revisionID, Result: result, CreatedBy: actor.UserID, CreatedAt: now})
	})
	return analysis, normalizeError(err)
}

func (service *Service) GetRevision(ctx context.Context, actor Actor, revisionID string) (domain.Analysis, error) {
	var analysis domain.Analysis
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		var err error
		analysis, err = repo.GetAnalysis(ctx, revisionID)
		if errors.Is(err, ErrNotFound) {
			return notFound("Document revision not found")
		}
		if err != nil {
			return err
		}
		_, err = repo.ProjectWorkspace(ctx, actor, analysis.Document.ProjectID, false)
		return err
	})
	return analysis, err
}

func (service *Service) GetCurrentAnalysis(ctx context.Context, actor Actor, projectID string) (domain.Analysis, error) {
	var analysis domain.Analysis
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		if _, err := repo.ProjectWorkspace(ctx, actor, projectID, false); err != nil {
			return err
		}
		var err error
		analysis, err = repo.GetCurrentAnalysis(ctx, projectID)
		if errors.Is(err, ErrNotFound) {
			return notFound("Current script document not found")
		}
		return err
	})
	return analysis, err
}

func (service *Service) ListDocuments(ctx context.Context, actor Actor, projectID string, limit, offset int) ([]domain.Document, int, error) {
	if limit < 1 || limit > 100 || offset < 0 {
		return nil, 0, invalid("Invalid pagination")
	}
	var items []domain.Document
	var total int
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		if _, err := repo.ProjectWorkspace(ctx, actor, projectID, false); err != nil {
			return err
		}
		var err error
		items, total, err = repo.ListDocuments(ctx, projectID, limit, offset)
		return err
	})
	return items, total, err
}

func (service *Service) projectWorkspace(ctx context.Context, actor Actor, projectID string, write bool) (string, error) {
	var workspaceID string
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		var err error
		workspaceID, err = repo.ProjectWorkspace(ctx, actor, projectID, write)
		return err
	})
	return workspaceID, err
}

func hashText(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

func invalid(message string) error {
	return &Error{Code: "validation_failed", Message: message, Status: 422}
}

func notFound(message string) error {
	return &Error{Code: "not_found", Message: message, Status: 404}
}

func normalizeError(err error) error {
	if errors.Is(err, platformcommand.ErrInputMismatch) {
		return &Error{Code: "resource_conflict", Message: "Idempotency key was already used with different input", Status: 409}
	}
	return err
}

package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
	promptapp "github.com/StephenQiu30/lanverse/backend/internal/prompt/application"
)

// QuotePromptCompiler consumes current authorized facts in the quote transaction.
type QuotePromptCompiler interface {
	Prepare(context.Context, identityapp.Principal, promptapp.TemplateRequest, promptapp.RuntimeContext) (promptapp.Preparation, error)
}

// PrepareFreeQuoteWithTemplate binds compilation to the original validated request.
// The adapter must verify canvas source against original input before compiling.
func PrepareFreeQuoteWithTemplate(input CreateFreeQuoteInput, catalog FreeQuoteCatalog, media []FreeQuoteMediaFact, now time.Time, candidate *uuid.UUID, compilation *promptapp.Preparation) (PreparedFreeQuote, error) {
	if input.PromptTemplate == nil {
		if compilation != nil {
			return PreparedFreeQuote{}, promptapp.ErrTemplateInvalid
		}
		return PrepareFreeQuote(input, catalog, media, now, candidate)
	}
	if err := input.Validate(); err != nil {
		return PreparedFreeQuote{}, err
	}
	if compilation == nil {
		return PreparedFreeQuote{}, promptapp.ErrTemplateUnavailable
	}
	evidence, err := bindPromptPreparation(input, *compilation)
	if err != nil {
		return PreparedFreeQuote{}, err
	}
	input.Prompt, input.PromptTemplate = compilation.FinalPrompt, nil
	prepared, err := PrepareFreeQuote(input, catalog, media, now, candidate)
	if err != nil {
		return PreparedFreeQuote{}, err
	}
	prepared.PromptPreparation = &evidence
	return prepared, nil
}

func bindPromptPreparation(input CreateFreeQuoteInput, compilation promptapp.Preparation) (domain.PromptPreparation, error) {
	request := input.PromptTemplate
	requestHash, err := request.Fingerprint()
	if err != nil {
		return domain.PromptPreparation{}, err
	}
	if compilation.ProjectID != input.ProjectID || compilation.OriginalPrompt != input.Prompt ||
		compilation.Operation != request.Operation || compilation.RequestSHA256 != requestHash ||
		compilation.UserPromptSHA256 != quotePromptHash(input.Prompt) || compilation.ContentSHA256 != quotePromptHash(compilation.FinalPrompt) {
		return domain.PromptPreparation{}, promptapp.ErrTemplateInvalid
	}
	evidence := domain.PromptPreparation{
		Version: 1, Operation: compilation.Operation, Policy: compilation.Policy,
		TemplateVersion: compilation.TemplateVersion, CustomizationRevision: compilation.CustomizationRevision,
		RequestSHA256: compilation.RequestSHA256, UserPromptSHA256: compilation.UserPromptSHA256, ContentSHA256: compilation.ContentSHA256,
	}
	if compilation.TemplateID != uuid.Nil {
		evidence.TemplateID = &compilation.TemplateID
	}
	if compilation.CustomizationID != uuid.Nil {
		evidence.CustomizationID = &compilation.CustomizationID
	}
	if evidence.Validate() != nil {
		return domain.PromptPreparation{}, promptapp.ErrTemplateInvalid
	}
	if evidence.Policy == "compiled" && (compilation.TemplateID != request.ExpectedTemplateID || compilation.CustomizationRevision != *request.ExpectedCustomizationRevision) {
		return domain.PromptPreparation{}, promptapp.ErrTemplateInvalid
	}
	if evidence.Policy == "bypass_video" && compilation.FinalPrompt != input.Prompt {
		return domain.PromptPreparation{}, promptapp.ErrTemplateInvalid
	}
	return evidence, nil
}

func quotePromptHash(content string) string {
	digest := sha256.Sum256([]byte(content))
	return hex.EncodeToString(digest[:])
}

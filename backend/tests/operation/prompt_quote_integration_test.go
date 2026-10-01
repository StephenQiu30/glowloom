package operation_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	pgoperation "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/postgres"
	operationapp "github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	pgprompt "github.com/StephenQiu30/lanverse/backend/internal/prompt/adapter/postgres"
	promptapp "github.com/StephenQiu30/lanverse/backend/internal/prompt/application"
	promptdomain "github.com/StephenQiu30/lanverse/backend/internal/prompt/domain"
)

func templateQuoteStore(database *gorm.DB) *pgoperation.Store {
	return pgoperation.NewStoreWithPromptCompiler(database, func(tx *gorm.DB) operationapp.QuotePromptCompiler {
		return promptapp.NewCompiler(pgprompt.NewStore(tx))
	})
}

func seedTemplateQuoteCatalog(t *testing.T, database *gorm.DB, projectID uuid.UUID) string {
	t.Helper()
	quoted, modelID, _ := seedQuoteCatalog(t, database, projectID, time.Now())
	if err := database.Exec(`INSERT INTO catalog.capability(id,key,output_type,modes,input_roles) VALUES(?,'text.structured','json',ARRAY['structured'],ARRAY['prompt']) ON CONFLICT(key) DO NOTHING`, uuid.New()).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`UPDATE catalog.model_profile SET capability='text.structured' WHERE id=?`, modelID).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`UPDATE catalog.model_profile_version SET modes=ARRAY['structured'],limits='{"max_outputs":1}' WHERE id=?`, quoted.ModelProfileVersionID).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`UPDATE catalog.price_rule_version SET unit='per_1k_chars',rule='{"base_micros":1}' WHERE id=?`, quoted.PriceRuleVersionID).Error; err != nil {
		t.Fatal(err)
	}
	return "quote-model-" + modelID.String()
}

func templateQuoteInput(projectID uuid.UUID, modelKey string) operationapp.CreateFreeQuoteInput {
	definition, _ := promptdomain.DefinitionFor("skill_draft")
	revision := int64(0)
	return operationapp.CreateFreeQuoteInput{ProjectID: projectID, RequestID: uuid.NewString(), FreeQuoteItemInput: operationapp.FreeQuoteItemInput{
		ModelKey: modelKey, Capability: "text.structured", Mode: "structured", Prompt: "用户原始技能要求", OutputCount: 1,
		PromptTemplate: &promptapp.TemplateRequest{Operation: definition.Operation, ExpectedTemplateID: definition.TemplateID, ExpectedCustomizationRevision: &revision},
	}}
}

func saveQuoteCustomization(t *testing.T, database *gorm.DB, actor identityapp.Principal, revision int64, content string) {
	t.Helper()
	definition, _ := promptdomain.DefinitionFor("skill_draft")
	_, err := promptapp.NewPreferences(pgprompt.NewStore(database), time.Now).Save(t.Context(), actor, promptapp.SaveInput{
		Operation: definition.Operation, Mode: promptdomain.Append, Content: content, BaseTemplateID: definition.TemplateID,
		ExpectedRevision: revision, IdempotencyKey: uuid.New(), RequestID: uuid.NewString(),
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTemplateQuoteFreezesPersonalRevisionAndReplaysAfterSettingsChange(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	modelKey := seedTemplateQuoteCatalog(t, database, projectID)
	saveQuoteCustomization(t, database, actor, 0, strings.Repeat("合成个人要求", 240))
	input := templateQuoteInput(projectID, modelKey)
	revision := int64(1)
	input.PromptTemplate.ExpectedCustomizationRevision = &revision
	store := templateQuoteStore(database)
	first, err := store.CreateFreeQuote(t.Context(), actor, input)
	if err != nil || first.PromptPreparation == nil || first.PromptPreparation.CustomizationRevision != 1 || !strings.Contains(first.FinalPrompt, "合成个人要求") || !strings.Contains(first.FinalPrompt, input.Prompt) {
		t.Fatalf("frozen template quote missing revision or input: %v", err)
	}
	characters := int64(utf8.RuneCountInString(first.FinalPrompt))
	if first.QuoteMicros != (characters+999)/1000 {
		t.Fatal("quote did not charge final compiled characters")
	}
	detail, err := store.ReadPublicTask(t.Context(), actor, projectID, first.OperationID)
	if err != nil || detail.PromptPreparation == nil || detail.PromptPreparation.ContentSHA256 != first.PromptPreparation.ContentSHA256 || len(detail.Inputs) != 1 || detail.Inputs[0].Text == nil || *detail.Inputs[0].Text != first.FinalPrompt {
		t.Fatal("refresh lost compiled quote evidence or unique frozen text", err)
	}
	saveQuoteCustomization(t, database, actor, 1, "后来保存的要求")
	replay, err := store.CreateFreeQuote(t.Context(), actor, input)
	if err != nil || replay.OperationID != first.OperationID || replay.FinalPrompt != first.FinalPrompt || replay.PromptPreparation.CustomizationRevision != 1 {
		t.Fatal("same request recomputed current personal preference", err)
	}
	input.RequestID = uuid.NewString()
	if _, err := store.CreateFreeQuote(t.Context(), actor, input); !errors.Is(err, promptapp.ErrTemplateChanged) {
		t.Fatal("new quote accepted stale personal revision", err)
	}
	revision = 2
	if _, err := store.CreateFreeQuote(t.Context(), actor, input); err != nil {
		t.Fatal("current personal revision could not be compiled", err)
	}
	if _, err := store.ConfirmSingleQuote(t.Context(), actor, operationapp.ConfirmSingleQuoteInput{ProjectID: projectID, OperationID: first.OperationID, RequestID: uuid.NewString()}); err != nil {
		t.Fatal("saved preference change invalidated immutable quote", err)
	}
}

func TestTemplateQuoteAuthorityFailureAndBatchAtomicity(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	modelKey := seedTemplateQuoteCatalog(t, database, projectID)
	input := templateQuoteInput(projectID, modelKey)
	store := templateQuoteStore(database)
	foreign, _ := operationStoreProject(t, database)
	if _, err := store.CreateFreeQuote(t.Context(), foreign, input); !errors.Is(err, pgoperation.ErrNotFound) {
		t.Fatal("foreign actor compiled project context", err)
	}
	if _, err := pgoperation.NewStore(database).CreateFreeQuote(t.Context(), actor, input); !errors.Is(err, promptapp.ErrTemplateUnavailable) {
		t.Fatal("missing compiler silently generated original input", err)
	}
	missingContext := input.FreeQuoteItemInput
	definition, _ := promptdomain.DefinitionFor("character_extract")
	selection := *missingContext.PromptTemplate
	selection.Operation, selection.ExpectedTemplateID = definition.Operation, definition.TemplateID
	missingContext.PromptTemplate = &selection
	if _, err := store.CreateFreeQuote(t.Context(), actor, operationapp.CreateFreeQuoteInput{ProjectID: projectID, RequestID: uuid.NewString(), FreeQuoteItemInput: missingContext}); !errors.Is(err, promptapp.ErrTemplateContextUnavailable) {
		t.Fatal("unavailable chapter entity was invented", err)
	}
	batchInput := operationapp.CreateBatchFreeQuoteInput{ProjectID: projectID, RequestID: uuid.NewString(), Items: []operationapp.FreeQuoteItemInput{input.FreeQuoteItemInput, missingContext}}
	batch, err := store.CreateBatchFreeQuote(t.Context(), actor, batchInput)
	if err != nil || batch.BatchID == nil || batch.Items[0].PromptPreparation == nil || batch.Items[1].ErrorCode != "context_unavailable" || batch.Items[1].OperationID != nil {
		t.Fatal("batch lost order or manufactured missing-context output", err)
	}
	changed := *input.PromptTemplate
	changed.ExpectedTemplateID = uuid.New()
	input.PromptTemplate = &changed
	if _, err := store.CreateFreeQuote(t.Context(), actor, input); !errors.Is(err, promptapp.ErrTemplateChanged) {
		t.Fatal("baseline change accepted", err)
	}
	if err := database.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateBatchFreeQuote(t.Context(), actor, batchInput); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatal("replay bypassed revoked actor", err)
	}
}

func TestTemplateQuoteAtomicRollbackAndRuntimePrivileges(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	input := templateQuoteInput(projectID, seedTemplateQuoteCatalog(t, database, projectID))
	stop := errors.New("synthetic quote transaction rollback")
	var attempted uuid.UUID
	err := database.Transaction(func(tx *gorm.DB) error {
		quote, err := templateQuoteStore(tx).CreateFreeQuote(t.Context(), actor, input)
		if err != nil {
			return err
		}
		attempted = quote.OperationID
		return stop
	})
	if !errors.Is(err, stop) {
		t.Fatal(err)
	}
	var count int64
	if err := database.Raw(`SELECT count(*) FROM operation.operation WHERE id=?`, attempted).Scan(&count).Error; err != nil || count != 0 {
		t.Fatal("rollback kept a frozen operation", err)
	}
	if err := database.Raw(`SELECT count(*) FROM operation.quote_request WHERE request_id=?`, input.RequestID).Scan(&count).Error; err != nil || count != 0 {
		t.Fatal("rollback kept idempotency receipt", err)
	}
	err = database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return err
		}
		quote, err := templateQuoteStore(tx).CreateFreeQuote(t.Context(), actor, input)
		if err != nil {
			return err
		}
		if quote.PromptPreparation == nil {
			return errors.New("runtime role lost template evidence")
		}
		encoded, err := json.Marshal(quote.PromptPreparation)
		if err != nil {
			return err
		}
		if err := tx.Exec(`SAVEPOINT immutable_prompt_check`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE operation.operation SET prompt_preparation=?::jsonb WHERE id=?`, string(encoded), quote.OperationID).Error; err == nil {
			return errors.New("runtime role can rewrite immutable prompt proof")
		}
		return tx.Exec(`ROLLBACK TO SAVEPOINT immutable_prompt_check`).Error
	})
	if err != nil {
		t.Fatal("runtime role failed template transaction or immutability guard", err)
	}
}

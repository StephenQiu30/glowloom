package prompt_test

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/prompt/domain"
)

func TestPromptCustomizationContracts(t *testing.T) {
	definitions := domain.Definitions()
	if len(definitions) != 9 {
		t.Fatalf("definitions = %d, want all nine source operations", len(definitions))
	}
	definition, ok := domain.DefinitionFor("short_drama_outline")
	if !ok || definition.TemplateID == uuid.Nil || definition.OutputContract == "" {
		t.Fatal("missing immutable baseline")
	}
	for _, mode := range []domain.Mode{domain.Inherit, domain.Append, domain.Rewrite} {
		t.Run(string(mode), func(t *testing.T) {
			content := "个人要求 {{章节数量}}"
			if mode == domain.Inherit {
				content = ""
			}
			customization := domain.Customization{ID: uuid.New(), Operation: definition.Operation, Mode: mode, Content: content, BaseTemplateID: definition.TemplateID, Revision: 1, UpdateTime: time.Now()}
			compiled, err := domain.Compile(definition.Operation, &customization, map[string]string{"章节数量": "3", "用户故事": "受保护剧情"})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(compiled.Content, definition.OutputContract) || !strings.Contains(compiled.Content, "受保护剧情") {
				t.Fatal("customization removed protected context or output contract")
			}
			if mode != domain.Inherit && !strings.Contains(compiled.Content, "个人要求 3") {
				t.Fatal("declared variable was not rendered")
			}
		})
	}
	for _, content := range []string{"{{未知变量}}", "{{章节数量", "章节数量}}", strings.Repeat("字", 12001)} {
		if err := domain.ValidateContent(definition.Operation, domain.Append, content); err == nil {
			t.Fatalf("invalid content accepted: length %d", len(content))
		}
	}
	if err := domain.ValidateContent("unknown", domain.Inherit, ""); err == nil {
		t.Fatal("unknown operation accepted")
	}
}

func TestPromptFrozenDefinitionCompilesHistoricalBaselineWithoutCurrentLookup(t *testing.T) {
	definition, _ := domain.DefinitionFor("skill_draft")
	definition.DefaultContent = "历史基线 {{历史变量}}"
	definition.Variables = []domain.Variable{{Label: "历史上下文", Placeholder: "{{历史变量}}"}}
	digest := sha256.Sum256([]byte(definition.DefaultContent + "\x00" + definition.OutputContract))
	definition.TemplateID = uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf("glowloom/prompt/%s/%x", definition.Operation, digest)))
	customization := domain.Customization{ID: uuid.New(), Operation: definition.Operation, Mode: domain.Append, Content: "历史偏好 {{历史变量}}", BaseTemplateID: definition.TemplateID, Revision: 1, UpdateTime: time.Now()}
	if customization.Validate() == nil {
		t.Fatal("fixture no longer distinguishes today's allowed variables from historical variables")
	}
	compiled, err := domain.CompileDefinition(definition, &customization, map[string]string{"历史变量": "原冻结值"})
	if err != nil || !strings.Contains(compiled.Content, "历史基线 原冻结值") || !strings.Contains(compiled.Content, "历史偏好 原冻结值") || !strings.Contains(compiled.Content, definition.OutputContract) {
		t.Fatal("frozen compiler looked up today's baseline or preferences", compiled, err)
	}
	for _, mutation := range []string{"content", "contract", "duplicate-variable", "unknown-version", "malformed-placeholder"} {
		t.Run(mutation, func(t *testing.T) {
			changed := definition
			switch mutation {
			case "content":
				changed.DefaultContent += "changed"
			case "contract":
				changed.OutputContract += "changed"
			case "duplicate-variable":
				changed.Variables = append(append([]domain.Variable(nil), definition.Variables...), definition.Variables[0])
			case "unknown-version":
				changed.TemplateVersion++
			case "malformed-placeholder":
				changed.Variables = []domain.Variable{{Label: "bad", Placeholder: "prefix{{历史变量}}"}}
			}
			if changed.ValidateFrozen() == nil {
				t.Fatal("invalid frozen baseline accepted")
			}
		})
	}
}

func TestPromptCompileRejectsUnboundedRuntimeContext(t *testing.T) {
	_, err := domain.Compile("short_drama_outline", nil, map[string]string{"用户故事": strings.Repeat("x", 1024*1024+1)})
	if err == nil {
		t.Fatal("oversized runtime context accepted")
	}
	_, err = domain.Compile("short_drama_outline", nil, map[string]string{"用户故事": string([]byte{0xff})})
	if err == nil {
		t.Fatal("invalid UTF-8 runtime context accepted")
	}
}

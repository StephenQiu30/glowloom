package prompt_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	promptapp "github.com/StephenQiu30/lanverse/backend/internal/prompt/application"
	"github.com/StephenQiu30/lanverse/backend/internal/prompt/domain"
)

type compilerReader struct {
	items []domain.Customization
	err   error
	calls int
}

func (r *compilerReader) ReadCustomizations(ctx context.Context, _ identityapp.Principal) ([]domain.Customization, error) {
	r.calls++
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return r.items, r.err
}

func templateRequest(operation string) promptapp.TemplateRequest {
	definition, _ := domain.DefinitionFor(operation)
	revision := int64(0)
	request := promptapp.TemplateRequest{Operation: operation, ExpectedTemplateID: definition.TemplateID, ExpectedCustomizationRevision: &revision}
	if operation == "short_drama_outline" {
		request.Outline = &promptapp.OutlineOptions{ChapterCount: "5", Structure: "单线推进", WordCount: "800", Perspective: "第三人称", Tone: "平稳叙事", CharacterScale: "3-4 个", ChapterLength: "中"}
	}
	return request
}

func templateRuntime(operation string) promptapp.RuntimeContext {
	runtime := promptapp.RuntimeContext{ProjectID: uuid.New(), Capability: "text.structured", Mode: "structured", UserPrompt: "本次用户要求必须保留", ServerValues: map[string]string{
		"项目名称": "合成项目", "项目画风": "写实", "章节名称": "合成章", "章节正文": "受保护章节正文", "角色名称": "合成人物", "角色设定": "受保护角色定义",
		"剧情": "受保护剧情", "用户要求": "禁止使用客户端覆盖", "画布资产": "[]", "角色版本": "[]", "单镜头时长规则": "5秒", "镜头数量规则": "3镜", "校验错误": "真实校验错误", "原始输出": "真实原始输出",
		"项目视觉": "已保存视觉", "首帧构图": "已保存构图", "表演起始状态": "静止", "负面要求": "无水印",
	}}
	if operation == "storyboard_first_frame" || operation == "character_turnaround" {
		runtime.Capability, runtime.Mode = "image.generate", "text_to_image"
	}
	if operation == "storyboard_video" {
		runtime.Capability, runtime.Mode = "video.generate", "text_to_video"
	}
	return runtime
}

func compilerActor() identityapp.Principal {
	return identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), Role: "producer"}
}

func TestTemplateCompilerPreservesAllDeclaredOperationsAndInput(t *testing.T) {
	for _, definition := range domain.Definitions() {
		t.Run(definition.Operation, func(t *testing.T) {
			reader := &compilerReader{}
			runtime := templateRuntime(definition.Operation)
			if definition.Operation == "storyboard_video" {
				reader.err = errors.New("video must not read personal preferences")
			}
			prepared, err := promptapp.NewCompiler(reader).Prepare(t.Context(), compilerActor(), templateRequest(definition.Operation), runtime)
			if err != nil {
				t.Fatal(err)
			}
			if prepared.ProjectID != runtime.ProjectID || prepared.OriginalPrompt != runtime.UserPrompt || !strings.Contains(prepared.FinalPrompt, runtime.UserPrompt) {
				t.Fatal("compiler discarded the authorized project or original user input")
			}
			if definition.Operation == "storyboard_video" {
				if prepared.FinalPrompt != runtime.UserPrompt || reader.calls != 0 || prepared.Policy != "bypass_video" {
					t.Fatal("video input was compiled or preferences were read")
				}
			} else if reader.calls != 1 || prepared.Policy != "compiled" || !strings.Contains(prepared.FinalPrompt, definition.OutputContract) {
				t.Fatal("missing live personal revision or protected output contract")
			}
		})
	}
}

func TestTemplateCompilerRejectsStaleAndIncompleteContexts(t *testing.T) {
	request := templateRequest("character_extract")
	runtime := templateRuntime(request.Operation)
	delete(runtime.ServerValues, "章节正文")
	reader := &compilerReader{}
	if _, err := promptapp.NewCompiler(reader).Prepare(t.Context(), compilerActor(), request, runtime); !errors.Is(err, promptapp.ErrTemplateContextUnavailable) {
		t.Fatal("missing authorized chapter context accepted", err)
	}
	runtime = templateRuntime(request.Operation)
	request.ExpectedTemplateID = uuid.New()
	if _, err := promptapp.NewCompiler(reader).Prepare(t.Context(), compilerActor(), request, runtime); !errors.Is(err, promptapp.ErrTemplateChanged) {
		t.Fatal("wrong immutable baseline accepted", err)
	}
	request = templateRequest("character_extract")
	reader.items = []domain.Customization{{ID: uuid.New(), Operation: request.Operation, Mode: domain.Rewrite, Content: "个人要求", BaseTemplateID: request.ExpectedTemplateID, Revision: 2, UpdateTime: time.Now()}}
	if _, err := promptapp.NewCompiler(reader).Prepare(t.Context(), compilerActor(), request, runtime); !errors.Is(err, promptapp.ErrTemplateChanged) {
		t.Fatal("stale personal revision accepted", err)
	}
	revision := int64(2)
	request.ExpectedCustomizationRevision = &revision
	prepared, err := promptapp.NewCompiler(reader).Prepare(t.Context(), compilerActor(), request, runtime)
	if err != nil || !strings.Contains(prepared.FinalPrompt, "受保护章节正文") || !strings.Contains(prepared.FinalPrompt, runtime.UserPrompt) {
		t.Fatal("rewrite removed protected chapter or this-turn instructions", err)
	}
	reader.items[0].BaseTemplateID = uuid.New()
	if _, err := promptapp.NewCompiler(reader).Prepare(t.Context(), compilerActor(), request, runtime); !errors.Is(err, promptapp.ErrTemplateChanged) {
		t.Fatal("outdated rewrite was silently rebased", err)
	}
}

func TestTemplateCompilerValidatesClosedOptionsAndOwnership(t *testing.T) {
	request := templateRequest("short_drama_outline")
	runtime := templateRuntime(request.Operation)
	reader := &compilerReader{}
	request.Outline.ChapterCount = "999"
	if _, err := promptapp.NewCompiler(reader).Prepare(t.Context(), compilerActor(), request, runtime); !errors.Is(err, promptapp.ErrTemplateInvalid) {
		t.Fatal("unsupported chapter option accepted", err)
	}
	request = templateRequest("skill_draft")
	request.Outline = templateRequest("short_drama_outline").Outline
	if _, err := promptapp.NewCompiler(reader).Prepare(t.Context(), compilerActor(), request, templateRuntime(request.Operation)); !errors.Is(err, promptapp.ErrTemplateInvalid) {
		t.Fatal("another operation accepted outline options", err)
	}
	request = templateRequest("skill_draft")
	request.ExpectedCustomizationRevision = nil
	if err := request.ValidateFor("text.structured"); !errors.Is(err, promptapp.ErrTemplateInvalid) {
		t.Fatal("missing explicit customization revision accepted", err)
	}
	request = templateRequest("skill_draft")
	actor := compilerActor()
	actor.Role = "unknown"
	if _, err := promptapp.NewCompiler(reader).Prepare(t.Context(), actor, request, templateRuntime(request.Operation)); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatal("unauthorized principal accepted", err)
	}
	if _, err := promptapp.NewCompiler(reader).Prepare(t.Context(), compilerActor(), request, templateRuntime("storyboard_first_frame")); !errors.Is(err, promptapp.ErrTemplateInvalid) {
		t.Fatal("wrong capability accepted", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := promptapp.NewCompiler(reader).Prepare(ctx, compilerActor(), request, templateRuntime(request.Operation)); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled compilation continued", err)
	}
}

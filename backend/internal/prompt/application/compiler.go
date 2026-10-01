package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/prompt/domain"
)

var (
	// ErrTemplateInvalid means a selected template or its bounded input is invalid.
	ErrTemplateInvalid = errors.New("invalid prompt template input")
	// ErrTemplateChanged means the selected baseline or personal revision is stale.
	ErrTemplateChanged = errors.New("prompt template changed")
	// ErrTemplateContextUnavailable means authoritative business context is missing.
	ErrTemplateContextUnavailable = errors.New("prompt template context unavailable")
	// ErrTemplateUnavailable means the injected preference reader is unavailable.
	ErrTemplateUnavailable = errors.New("prompt template reader unavailable")
)

// OutlineOptions preserves the seven choices offered by the fixed source page.
type OutlineOptions struct {
	ChapterCount   string `json:"chapter_count" binding:"required" enums:"3,5,8,10"`
	Structure      string `json:"structure" binding:"required" enums:"单线推进,双线并行,群像多线,反转嵌套"`
	WordCount      string `json:"word_count" binding:"required" enums:"500,800,1200,2000"`
	Perspective    string `json:"perspective" binding:"required" enums:"第三人称,第一人称,多视角"`
	Tone           string `json:"tone" binding:"required" enums:"平稳叙事,轻松喜剧,紧张悬疑,热血成长,甜宠治愈"`
	CharacterScale string `json:"character_scale" binding:"required" enums:"2 个,3-4 个,5-6 个"`
	ChapterLength  string `json:"chapter_length" binding:"required" enums:"短,中,长"`
}

func (o OutlineOptions) valid() bool {
	for _, choice := range []struct {
		value   string
		allowed []string
	}{
		{o.ChapterCount, []string{"3", "5", "8", "10"}},
		{o.Structure, []string{"单线推进", "双线并行", "群像多线", "反转嵌套"}},
		{o.WordCount, []string{"500", "800", "1200", "2000"}},
		{o.Perspective, []string{"第三人称", "第一人称", "多视角"}},
		{o.Tone, []string{"平稳叙事", "轻松喜剧", "紧张悬疑", "热血成长", "甜宠治愈"}},
		{o.CharacterScale, []string{"2 个", "3-4 个", "5-6 个"}},
		{o.ChapterLength, []string{"短", "中", "长"}},
	} {
		if !slices.Contains(choice.allowed, choice.value) {
			return false
		}
	}
	return true
}

// TemplateRequest selects one immutable baseline without accepting runtime overrides.
type TemplateRequest struct {
	Operation                     string          `json:"operation" binding:"required" enums:"chapter_assets_extract,character_extract,character_turnaround,storyboard_plan,storyboard_repair,storyboard_first_frame,storyboard_video,short_drama_outline,skill_draft"`
	ExpectedTemplateID            uuid.UUID       `json:"expected_template_id" binding:"required"`
	ExpectedCustomizationRevision *int64          `json:"expected_customization_revision" binding:"required" minimum:"0" maximum:"2147483647"`
	Outline                       *OutlineOptions `json:"outline,omitempty"`
}

// ValidateFor rejects unsupported operations, capabilities, and source option values.
func (r TemplateRequest) ValidateFor(capability string) error {
	if _, ok := domain.DefinitionFor(r.Operation); !ok || r.ExpectedTemplateID == uuid.Nil ||
		r.ExpectedCustomizationRevision == nil || *r.ExpectedCustomizationRevision < 0 || *r.ExpectedCustomizationRevision > math.MaxInt32 {
		return ErrTemplateInvalid
	}
	if r.Operation == "short_drama_outline" {
		if r.Outline == nil || !r.Outline.valid() {
			return ErrTemplateInvalid
		}
	} else if r.Outline != nil {
		return ErrTemplateInvalid
	}
	if capability != templateCapability(r.Operation) {
		return ErrTemplateInvalid
	}
	return nil
}

func templateCapability(operation string) string {
	switch operation {
	case "storyboard_first_frame", "character_turnaround":
		return "image.generate"
	case "storyboard_video":
		return "video.generate"
	default:
		return "text.structured"
	}
}

// Fingerprint is the canonical explicit selection used to bind a prepared prompt.
func (r TemplateRequest) Fingerprint() (string, error) {
	if err := r.ValidateFor(templateCapability(r.Operation)); err != nil {
		return "", err
	}
	body, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("encode template selection: %w", err)
	}
	return contentHash(string(body)), nil
}

// RuntimeContext contains only already authorized application facts.
// ServerValues is deliberately absent from the public request contract.
type RuntimeContext struct {
	ProjectID    uuid.UUID
	Capability   string
	Mode         string
	UserPrompt   string
	ServerValues map[string]string
}

// Preparation is a compiled result for an immutable quote, not an execution receipt.
type Preparation struct {
	ProjectID             uuid.UUID
	OriginalPrompt        string
	FinalPrompt           string
	Operation             string
	Policy                string
	TemplateID            uuid.UUID
	TemplateVersion       int
	CustomizationID       uuid.UUID
	CustomizationRevision int64
	RequestSHA256         string
	UserPromptSHA256      string
	ContentSHA256         string
}

// CustomizationReader is the only preference capability consumed by the compiler.
type CustomizationReader interface {
	ReadCustomizations(context.Context, identityapp.Principal) ([]domain.Customization, error)
}

// Compiler retains the source video bypass and freezes only explicitly selected policy.
type Compiler struct{ reader CustomizationReader }

// NewCompiler injects a reader scoped by the calling quote transaction.
func NewCompiler(reader CustomizationReader) *Compiler { return &Compiler{reader: reader} }

// Prepare validates current selection and authorized context before producing final text.
func (c *Compiler) Prepare(ctx context.Context, actor identityapp.Principal, request TemplateRequest, runtime RuntimeContext) (Preparation, error) {
	if err := ctx.Err(); err != nil {
		return Preparation{}, fmt.Errorf("prepare template context: %w", err)
	}
	if !canCustomize(actor) {
		return Preparation{}, identityapp.ErrForbidden
	}
	if err := request.ValidateFor(runtime.Capability); err != nil {
		return Preparation{}, err
	}
	if runtime.ProjectID == uuid.Nil || strings.TrimSpace(runtime.Mode) == "" || !validQuotePrompt(runtime.UserPrompt) {
		return Preparation{}, ErrTemplateInvalid
	}
	definition, _ := domain.DefinitionFor(request.Operation)
	if definition.TemplateID != request.ExpectedTemplateID {
		return Preparation{}, ErrTemplateChanged
	}
	requestHash, err := request.Fingerprint()
	if err != nil {
		return Preparation{}, err
	}
	prepared := Preparation{ProjectID: runtime.ProjectID, OriginalPrompt: runtime.UserPrompt, Operation: request.Operation,
		RequestSHA256: requestHash, UserPromptSHA256: contentHash(runtime.UserPrompt)}
	if request.Operation == "storyboard_video" {
		prepared.FinalPrompt, prepared.Policy = runtime.UserPrompt, "bypass_video"
		prepared.ContentSHA256 = prepared.UserPromptSHA256
		return prepared, nil
	}
	values, err := runtimeValues(request, runtime)
	if err != nil {
		return Preparation{}, err
	}
	if c == nil || c.reader == nil {
		return Preparation{}, ErrTemplateUnavailable
	}
	customizations, err := c.reader.ReadCustomizations(ctx, actor)
	if err != nil {
		if !errors.Is(err, identityapp.ErrForbidden) && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			return Preparation{}, fmt.Errorf("%w: read template customization: %w", ErrTemplateUnavailable, err)
		}
		return Preparation{}, fmt.Errorf("read template customization: %w", err)
	}
	var selected *domain.Customization
	for _, customization := range customizations {
		if customization.Operation != request.Operation {
			continue
		}
		if selected != nil || customization.Validate() != nil {
			return Preparation{}, ErrTemplateInvalid
		}
		selected = &customization
	}
	revision := int64(0)
	if selected != nil {
		revision = selected.Revision
		if selected.Mode == domain.Rewrite && selected.BaseTemplateID != definition.TemplateID {
			return Preparation{}, ErrTemplateChanged
		}
	}
	if revision != *request.ExpectedCustomizationRevision {
		return Preparation{}, ErrTemplateChanged
	}
	compiled, err := domain.Compile(request.Operation, selected, values)
	if err != nil || !validQuotePrompt(compiled.Content) {
		return Preparation{}, ErrTemplateInvalid
	}
	prepared.FinalPrompt, prepared.Policy = compiled.Content, "compiled"
	prepared.TemplateID, prepared.TemplateVersion = definition.TemplateID, definition.TemplateVersion
	prepared.CustomizationID, prepared.CustomizationRevision = compiled.CustomizationID, compiled.CustomizationRevision
	prepared.ContentSHA256 = contentHash(prepared.FinalPrompt)
	return prepared, nil
}

func validQuotePrompt(content string) bool {
	return strings.TrimSpace(content) != "" && len(content) <= 64*1024 && utf8.ValidString(content) && !strings.ContainsRune(content, 0)
}

func runtimeValues(request TemplateRequest, runtime RuntimeContext) (map[string]string, error) {
	values := make(map[string]string)
	var required []string
	switch request.Operation {
	case "short_drama_outline":
		options := request.Outline
		values["用户故事"] = runtime.UserPrompt
		values["章节数量"], values["叙事结构"], values["每章字数"] = options.ChapterCount, options.Structure, options.WordCount
		values["叙事视角"], values["整体基调"] = options.Perspective, options.Tone
		values["角色规模"], values["章节篇幅"] = options.CharacterScale, options.ChapterLength
	case "skill_draft":
		values["用户想法"] = runtime.UserPrompt
	case "chapter_assets_extract", "character_extract":
		required = []string{"项目名称", "项目画风", "章节名称", "章节正文"}
	case "character_turnaround":
		required = []string{"角色名称", "角色设定", "项目画风"}
	case "storyboard_plan", "storyboard_repair":
		required = []string{"项目名称", "项目画风", "剧情", "画布资产", "角色版本", "单镜头时长规则", "镜头数量规则"}
		if request.Operation == "storyboard_repair" {
			required = append(required, "校验错误", "原始输出")
		}
		values["用户要求"] = runtime.UserPrompt
	case "storyboard_first_frame":
		required = []string{"项目视觉", "首帧构图", "表演起始状态", "负面要求"}
	default:
		return nil, ErrTemplateInvalid
	}
	for _, key := range required {
		value, present := runtime.ServerValues[key]
		if !present || strings.TrimSpace(value) == "" {
			return nil, ErrTemplateContextUnavailable
		}
		if !utf8.ValidString(value) || strings.ContainsRune(value, 0) || len(value) > 64*1024 {
			return nil, ErrTemplateInvalid
		}
		values[key] = value
	}
	values["本次用户要求"] = runtime.UserPrompt
	return values, nil
}

func contentHash(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}

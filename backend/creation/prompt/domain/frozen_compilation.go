package domain

import (
	"slices"
	"strings"
	"unicode/utf8"
)

// OutlineOptions preserves the seven accepted choices of the source editor.
type OutlineOptions struct {
	ChapterCount   string `json:"chapter_count" binding:"required" enums:"3,5,8,10"`
	Structure      string `json:"structure" binding:"required" enums:"单线推进,双线并行,群像多线,反转嵌套"`
	WordCount      string `json:"word_count" binding:"required" enums:"500,800,1200,2000"`
	Perspective    string `json:"perspective" binding:"required" enums:"第三人称,第一人称,多视角"`
	Tone           string `json:"tone" binding:"required" enums:"平稳叙事,轻松喜剧,紧张悬疑,热血成长,甜宠治愈"`
	CharacterScale string `json:"character_scale" binding:"required" enums:"2 个,3-4 个,5-6 个"`
	ChapterLength  string `json:"chapter_length" binding:"required" enums:"短,中,长"`
}

// Valid rejects implicit defaults and unknown editor options.
func (o OutlineOptions) Valid() bool {
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

// FrozenTemplate retains actual server baseline and the exact personal layer.
// A nil Customization records absence at the source transaction's lock boundary.
type FrozenTemplate struct {
	Definition    Definition      `json:"definition"`
	Customization *Customization  `json:"customization,omitempty"`
	RequestSHA256 string          `json:"request_sha256"`
	Outline       *OutlineOptions `json:"outline,omitempty"`
}

// FrozenCompilationInputs are captured alongside the source draft, never resolved
// from today's project, definition or personal preferences during compilation.
type FrozenCompilationInputs struct {
	CompilerVersion string          `json:"compiler_version"`
	ProjectName     string          `json:"project_name"`
	StyleType       string          `json:"style_type"`
	Template        *FrozenTemplate `json:"template,omitempty"`
}

// Validate checks closed snapshot content. Unknown nonempty compiler versions can
// still be read for recovery; actual compilation separately rejects them.
func (f FrozenCompilationInputs) Validate() error {
	for _, value := range []string{f.CompilerVersion, f.ProjectName, f.StyleType} {
		if strings.TrimSpace(value) == "" || !utf8.ValidString(value) || strings.ContainsRune(value, 0) || len(value) > 64<<10 {
			return ErrFrozenInputInvalid
		}
	}
	if f.Template == nil {
		return nil
	}
	t := f.Template
	if t.Definition.ValidateFrozen() != nil || !validFrozenHash(t.RequestSHA256) {
		return ErrFrozenInputInvalid
	}
	if (t.Definition.Operation == "short_drama_outline") != (t.Outline != nil) || (t.Outline != nil && !t.Outline.Valid()) {
		return ErrFrozenInputInvalid
	}
	if t.Customization != nil && (t.Customization.ValidateAgainst(t.Definition) != nil || (t.Customization.Mode == Rewrite && t.Customization.BaseTemplateID != t.Definition.TemplateID)) {
		return ErrFrozenInputInvalid
	}
	if t.Definition.Operation == "storyboard_video" && t.Customization != nil {
		return ErrFrozenInputInvalid
	}
	return nil
}

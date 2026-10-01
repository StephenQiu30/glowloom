// Package domain owns immutable prompt baselines and bounded workspace customization.
package domain

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// ErrInvalidCustomization means a prompt preference violates its operation contract.
var ErrInvalidCustomization = errors.New("invalid prompt customization")

// Mode selects the personal creative layer without changing protected runtime context.
type Mode string

// Prompt modes preserve the baseline, append instructions, or replace its creative layer.
const (
	Inherit Mode = "inherit"
	Append  Mode = "append"
	Rewrite Mode = "rewrite"
)

// Variable is one declared template placeholder.
type Variable struct {
	Label       string `json:"label"`
	Placeholder string `json:"placeholder"`
}

// Definition is the server-owned baseline and output contract of an operation.
type Definition struct {
	Operation       string     `json:"operation"`
	Label           string     `json:"label"`
	Category        string     `json:"category"`
	Description     string     `json:"description"`
	OutputType      string     `json:"output_type"`
	SchemaKey       string     `json:"schema_key"`
	Variables       []Variable `json:"variables"`
	OutputContract  string     `json:"output_contract"`
	DefaultContent  string     `json:"content"`
	TemplateID      uuid.UUID  `json:"template_id"`
	TemplateVersion int        `json:"template_version"`
}

// Definitions returns independent snapshots of all nine fixed source baselines.
func Definitions() []Definition {
	definitions := defaultPromptDefinitions()
	for index := range definitions {
		definition := &definitions[index]
		definition.OutputContract = promptOutputContract(definition.Operation)
		definition.Variables = slices.Clone(definition.Variables)
		if definition.Variables == nil {
			definition.Variables = []Variable{}
		}
		hash := sha256.Sum256([]byte(definition.DefaultContent + "\x00" + definition.OutputContract))
		definition.TemplateID = uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf("lanverse/prompt/%s/%x", definition.Operation, hash)))
		definition.TemplateVersion = 1
	}
	return definitions
}

// DefinitionFor returns a fresh server-owned snapshot for one allowed operation.
func DefinitionFor(operation string) (Definition, bool) {
	for _, definition := range Definitions() {
		if definition.Operation == operation {
			return definition, true
		}
	}
	return Definition{}, false
}

// Customization contains only the personal creative layer and its stable revision.
type Customization struct {
	ID             uuid.UUID `json:"id"`
	Operation      string    `json:"operation"`
	Mode           Mode      `json:"mode"`
	Content        string    `json:"content"`
	BaseTemplateID uuid.UUID `json:"base_template_id"`
	Revision       int64     `json:"revision"`
	UpdateTime     time.Time `json:"update_time"`
}

var placeholderPattern = regexp.MustCompile(`\{\{[^{}]+\}\}`)

// ValidateContent rejects unsupported operations, modes, and template variables.
func ValidateContent(operation string, mode Mode, content string) error {
	definition, ok := DefinitionFor(operation)
	if !ok || !utf8.ValidString(content) || utf8.RuneCountInString(content) > 12000 {
		return ErrInvalidCustomization
	}
	switch mode {
	case Inherit:
		if content != "" {
			return ErrInvalidCustomization
		}
	case Append, Rewrite:
		if strings.TrimSpace(content) == "" {
			return ErrInvalidCustomization
		}
	default:
		return ErrInvalidCustomization
	}
	allowed := make(map[string]bool, len(definition.Variables))
	for _, variable := range definition.Variables {
		allowed[variable.Placeholder] = true
	}
	for _, placeholder := range placeholderPattern.FindAllString(content, -1) {
		if !allowed[placeholder] {
			return ErrInvalidCustomization
		}
	}
	remainder := placeholderPattern.ReplaceAllString(content, "")
	if strings.Contains(remainder, "{{") || strings.Contains(remainder, "}}") {
		return ErrInvalidCustomization
	}
	return nil
}

// Validate checks the stored preference without silently rebasing an older rewrite.
func (c Customization) Validate() error {
	if c.ID == uuid.Nil || c.BaseTemplateID == uuid.Nil || c.Revision < 1 || c.UpdateTime.IsZero() {
		return ErrInvalidCustomization
	}
	return ValidateContent(c.Operation, c.Mode, c.Content)
}

// Compiled is the immutable prompt evidence to freeze in a generating operation.
type Compiled struct {
	Content               string
	TemplateID            uuid.UUID
	CustomizationID       uuid.UUID
	CustomizationRevision int64
}

// Compile renders the creative layer once and appends server-owned runtime constraints.
// Callers supply authorized project context; preferences never grant execution rights.
func Compile(operation string, customization *Customization, values map[string]string) (Compiled, error) {
	definition, ok := DefinitionFor(operation)
	if !ok {
		return Compiled{}, ErrInvalidCustomization
	}
	const maxCompiledBytes = 1024 * 1024
	contextBytes := 0
	for key, value := range values {
		if !utf8.ValidString(key) || !utf8.ValidString(value) || len(key) > maxCompiledBytes-contextBytes {
			return Compiled{}, ErrInvalidCustomization
		}
		contextBytes += len(key)
		if len(value) > maxCompiledBytes-contextBytes {
			return Compiled{}, ErrInvalidCustomization
		}
		contextBytes += len(value)
	}
	creative := definition.DefaultContent
	compiled := Compiled{TemplateID: definition.TemplateID}
	if customization != nil {
		if customization.Operation != operation || customization.Validate() != nil {
			return Compiled{}, ErrInvalidCustomization
		}
		compiled.CustomizationID, compiled.CustomizationRevision = customization.ID, customization.Revision
		switch customization.Mode {
		case Append:
			creative += "\n\n【用户个性化创作要求】\n" + customization.Content
		case Rewrite:
			creative = customization.Content
		case Inherit:
		}
	}
	rendered := placeholderPattern.ReplaceAllStringFunc(creative, func(placeholder string) string {
		key := strings.TrimSuffix(strings.TrimPrefix(placeholder, "{{"), "}}")
		return strings.TrimSpace(values[key])
	})
	parts := []string{strings.TrimSpace(rendered)}
	if protected := strings.TrimSpace(protectedPromptContext(operation, values)); protected != "" {
		parts = append(parts, protected)
	}
	// Even a rewrite omitting placeholders cannot remove server-provided values.
	for _, variable := range definition.Variables {
		key := strings.TrimSuffix(strings.TrimPrefix(variable.Placeholder, "{{"), "}}")
		if value := strings.TrimSpace(values[key]); value != "" {
			parts = append(parts, "【"+variable.Label+"】\n"+value)
		}
	}
	parts = append(parts, "【受保护输出契约】\n"+definition.OutputContract)
	compiled.Content = strings.Join(parts, "\n\n")
	if len(compiled.Content) > maxCompiledBytes {
		return Compiled{}, ErrInvalidCustomization
	}
	return compiled, nil
}

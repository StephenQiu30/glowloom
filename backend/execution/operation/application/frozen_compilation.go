package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// Frozen compilation errors retain the original template and reference meanings.
var (
	ErrFrozenTemplateInvalid            = errors.New("frozen template invalid")
	ErrFrozenTemplateChanged            = errors.New("frozen template changed")
	ErrFrozenTemplateContextUnavailable = errors.New("frozen template context unavailable")
	ErrFrozenCompilerUnsupported        = errors.New("frozen compiler unsupported")
	ErrFrozenReferenceUnavailable       = errors.New("frozen reference unavailable")
)

// FrozenCatalogContext contains only Execution's immutable selected catalog facts.
type FrozenCatalogContext struct {
	ModelVersionID uuid.UUID `json:"model_version_id"`
	ModelKey       string    `json:"model_key"`
	Capability     string    `json:"capability"`
	Mode           string    `json:"mode"`
}

// SHA256 hashes the original selected catalog facts without numeric conversion.
func (c FrozenCatalogContext) SHA256() (string, error) {
	if c.ModelVersionID == uuid.Nil {
		return "", ErrFrozenInputInvalid
	}
	for _, value := range []string{c.ModelKey, c.Capability, c.Mode} {
		if value == "" || len(value) > 256 || strings.TrimSpace(value) != value || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
			return "", ErrFrozenInputInvalid
		}
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return compilationHash(raw), nil
}

// CompileFrozenInputRequest binds an immutable handoff to the selected model context.
type CompileFrozenInputRequest struct {
	Key            FrozenInputKey       `json:"key"`
	CatalogContext FrozenCatalogContext `json:"catalog_context"`
}

// FrozenCompiledMedia records source order and exact optional subject evidence.
type FrozenCompiledMedia struct {
	SeqNo                int32      `json:"seq_no"`
	MediaAssetID         uuid.UUID  `json:"media_asset_id"`
	Role                 string     `json:"role"`
	SubjectBindingID     *uuid.UUID `json:"subject_binding_id,omitempty"`
	SubjectBindingSHA256 string     `json:"subject_binding_sha256,omitempty"`
}

// FrozenReferenceMapping preserves the compiler's original reference ordering.
type FrozenReferenceMapping struct {
	ReferenceID string `json:"reference_id"`
	Label       string `json:"label"`
	MediaSeqNos []int  `json:"media_seq_nos"`
	Role        string `json:"role"`
}

// FrozenTemplateEvidence retains the exact template selection and prompt digests.
type FrozenTemplateEvidence struct {
	Version               int        `json:"version"`
	Operation             string     `json:"operation"`
	Policy                string     `json:"policy"`
	TemplateID            *uuid.UUID `json:"template_id,omitempty"`
	TemplateVersion       int        `json:"template_version,omitempty"`
	CustomizationID       *uuid.UUID `json:"customization_id,omitempty"`
	CustomizationRevision int64      `json:"customization_revision,omitempty"`
	RequestSHA256         string     `json:"request_sha256"`
	UserPromptSHA256      string     `json:"user_prompt_sha256"`
	ContentSHA256         string     `json:"content_sha256"`
}

// FrozenInputCompilation is immutable source evidence, never a spending permit.
// The compiler identifier retains the already persisted original wire value.
type FrozenInputCompilation struct {
	Key               FrozenInputKey           `json:"key"`
	CatalogContext    FrozenCatalogContext     `json:"catalog_context"`
	ContextSHA256     string                   `json:"context_sha256"`
	CompilationSHA256 string                   `json:"compilation_sha256"`
	ModelVersionID    uuid.UUID                `json:"model_version_id"`
	Mode              string                   `json:"mode"`
	OriginalPrompt    string                   `json:"original_prompt"`
	FinalPrompt       string                   `json:"final_prompt"`
	ParamsJSON        json.RawMessage          `json:"params_json"`
	OutputCount       int32                    `json:"output_count"`
	Media             []FrozenCompiledMedia    `json:"media"`
	CompilerVersion   string                   `json:"compiler_version"`
	ReferenceMapping  []FrozenReferenceMapping `json:"reference_mapping"`
	TemplateEvidence  *FrozenTemplateEvidence  `json:"template_evidence,omitempty"`
}

func compilationHash(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

// Validate binds the full result to its original key/context and canonical digest.
func (c FrozenInputCompilation) Validate(request CompileFrozenInputRequest) error {
	contextHash, err := request.CatalogContext.SHA256()
	if err != nil || c.Key != request.Key || c.CatalogContext != request.CatalogContext || c.ContextSHA256 != contextHash || c.ModelVersionID != request.CatalogContext.ModelVersionID || c.Mode != request.CatalogContext.Mode || c.OutputCount < 1 || c.OutputCount > 8 || !projectWorkHash(c.CompilationSHA256) {
		return ErrFrozenInputConflict
	}
	if c.CompilerVersion != "prompt-reference-v1" {
		return ErrFrozenCompilerUnsupported
	}
	if _, err := writefence.CanonicalReceipt(c.ParamsJSON); err != nil {
		return ErrFrozenInputInvalid
	}
	for i, media := range c.Media {
		if media.SeqNo != int32(i+1) || media.MediaAssetID == uuid.Nil || media.Role == "" || (media.SubjectBindingID == nil) != (media.SubjectBindingSHA256 == "") {
			return ErrFrozenInputInvalid
		}
		if media.SubjectBindingID != nil && (*media.SubjectBindingID == uuid.Nil || !projectWorkHash(media.SubjectBindingSHA256)) {
			return ErrFrozenInputInvalid
		}
	}
	for _, mapping := range c.ReferenceMapping {
		if mapping.ReferenceID == "" || mapping.Label == "" || mapping.Role == "" {
			return ErrFrozenInputInvalid
		}
		for _, seq := range mapping.MediaSeqNos {
			if seq < 1 || seq > len(c.Media) {
				return ErrFrozenInputInvalid
			}
		}
	}
	if evidence := c.TemplateEvidence; evidence != nil {
		if evidence.Version != 1 || !projectWorkHash(evidence.RequestSHA256) || evidence.UserPromptSHA256 != compilationHash([]byte(c.OriginalPrompt)) || evidence.ContentSHA256 != compilationHash([]byte(c.FinalPrompt)) {
			return ErrFrozenInputConflict
		}
	}
	expected := c.CompilationSHA256
	c.CompilationSHA256 = ""
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	canonical, err := writefence.CanonicalReceipt(raw)
	if err != nil {
		return err
	}
	if compilationHash(canonical) != expected {
		return ErrFrozenInputConflict
	}
	return nil
}

// FrozenInputCompiler resolves immutable source evidence without authorizing spend.
type FrozenInputCompiler interface {
	CompileFrozenInput(context.Context, CompileFrozenInputRequest) (FrozenInputCompilation, error)
}

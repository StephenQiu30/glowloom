package application

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// AssetCreationInput creates a new owning project and its actual primary document.
type AssetCreationInput struct {
	Kind        string `json:"kind,omitempty" enums:"canvas,short_drama"`
	Name        string `json:"name,omitempty"`
	TemplateKey string `json:"template_key,omitempty"`
}

// CreatedAssetCanvas identifies one new object, never the reusable system template.
type CreatedAssetCanvas struct {
	ID              uuid.UUID `json:"id"`
	ProjectID       uuid.UUID `json:"project_id"`
	Name            string    `json:"name"`
	Kind            string    `json:"kind"`
	Collection      string    `json:"collection"`
	Revision        Revision  `json:"revision" swaggertype:"string"`
	ProjectRevision int64     `json:"project_revision"`
	TemplateKey     string    `json:"template_key"`
	IsTemplate      bool      `json:"is_template"`
}

// AssetTemplateCatalog is the versioned system metadata exposed to asset cards.
type AssetTemplateCatalog struct {
	Items []domain.SystemAssetTemplate `json:"items"`
}

// AssetProject is the minimal trusted result of workspace's atomic creator.
type AssetProject struct{ ProjectID, CanvasID uuid.UUID }

// AssetCreationAccess preserves workspace scope, audit, zero-budget and creation ownership.
type AssetCreationAccess interface {
	AuthorizeAssetCreation(context.Context, identity.Principal) error
	CreateAssetProject(context.Context, identity.Principal, uuid.UUID, string) (AssetProject, error)
}

// AssetCreationStore commits the project, its seeded canvas and the outer receipt together.
type AssetCreationStore interface {
	CreateAsset(context.Context, identity.Principal, uuid.UUID, AssetCreationInput) (CreatedAssetCanvas, error)
}

// AssetCreator validates direct creation without requiring an existing project.
type AssetCreator struct{ store AssetCreationStore }

// NewAssetCreator injects the transaction owner.
func NewAssetCreator(store AssetCreationStore) *AssetCreator { return &AssetCreator{store: store} }

// NormalizeAssetCreation rejects unknown or mismatched recipes before any writes.
func NormalizeAssetCreation(input AssetCreationInput) (AssetCreationInput, error) {
	kind, err := domain.ParseDocumentKind(input.Kind)
	if err != nil {
		return input, err
	}
	input.Kind = kind
	defaultName := "未命名画布"
	if kind == "short_drama" {
		defaultName = "未命名短剧"
	}
	if input.TemplateKey != "" {
		template, err := domain.SystemAssetTemplateByKey(input.TemplateKey)
		if err != nil || template.Kind != kind {
			return input, domain.ErrInvalidCommand
		}
		defaultName = template.Name
	}
	if input.Name == "" {
		input.Name = defaultName
	}
	if strings.TrimSpace(input.Name) != input.Name || !utf8.ValidString(input.Name) || utf8.RuneCountInString(input.Name) > 50 || input.Name == "" {
		return input, domain.ErrInvalidCommand
	}
	for _, r := range input.Name {
		if unicode.IsControl(r) {
			return input, domain.ErrInvalidCommand
		}
	}
	return input, nil
}

// Create admits only a normalized revision-fenced new object under a UUID key.
func (s *AssetCreator) Create(ctx context.Context, actor identity.Principal, key uuid.UUID, input AssetCreationInput) (CreatedAssetCanvas, error) {
	if s == nil || s.store == nil || key == uuid.Nil {
		return CreatedAssetCanvas{}, domain.ErrInvalidCommand
	}
	input, err := NormalizeAssetCreation(input)
	if err != nil {
		return CreatedAssetCanvas{}, err
	}
	return s.store.CreateAsset(ctx, actor, key, input)
}

package application

import (
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// ErrAdapterNotFound rejects unknown or unavailable installed adapter identities.
var ErrAdapterNotFound = errors.New("provider adapter not found")

// ProtocolAvailability checks the existing installed driver registry at the composition root.
type ProtocolAvailability func(string) bool

// AdapterMetadataAccess checks the current platform session without loading secrets.
type AdapterMetadataAccess interface {
	AuthorizeAdapterMetadata(context.Context, identityapp.Principal) error
}

// AdapterMetadata is an explicitly value-free administrative projection.
type AdapterMetadata struct {
	AdapterKey        string            `json:"adapter_key"`
	DisplayName       string            `json:"display_name"`
	LogoKey           string            `json:"logo_key"`
	Capabilities      []string          `json:"capabilities"`
	ProtocolIDs       []string          `json:"protocol_ids"`
	CredentialFields  []CredentialField `json:"credential_fields"`
	CheckCapabilities []string          `json:"check_capabilities"`
}

// AdapterMetadataQuery combines the existing specification and credential contracts.
type AdapterMetadataQuery struct {
	access    AdapterMetadataAccess
	fields    CredentialFieldCatalog
	installed ProtocolAvailability
}

// NewAdapterMetadataQuery injects live authorization and installed protocol availability.
func NewAdapterMetadataQuery(access AdapterMetadataAccess, fields CredentialFieldCatalog, installed ProtocolAvailability) *AdapterMetadataQuery {
	return &AdapterMetadataQuery{access: access, fields: fields, installed: installed}
}

// List returns only safe descriptors of adapters with installed implementations.
func (q *AdapterMetadataQuery) List(ctx context.Context, actor identityapp.Principal) ([]AdapterMetadata, error) {
	if !IsPlatformAdmin(actor) || actor.OrgID == uuid.Nil || q == nil || q.access == nil {
		return nil, identityapp.ErrForbidden
	}
	if err := q.access.AuthorizeAdapterMetadata(ctx, actor); err != nil {
		return nil, err
	}
	items := []AdapterMetadata{}
	if q.fields == nil || q.installed == nil {
		return items, nil
	}
	for _, spec := range domain.AdapterSpecifications() {
		fields, ok := q.fields.Fields(spec.AdapterKey)
		if !ok || !spec.IsManagedCloud {
			continue
		}
		protocols := []string{}
		for _, id := range spec.ProtocolIDs {
			if q.installed(id) {
				protocols = append(protocols, id)
			}
		}
		if len(protocols) == 0 {
			continue
		}
		items = append(items, AdapterMetadata{AdapterKey: spec.AdapterKey, DisplayName: spec.DisplayName, LogoKey: spec.LogoKey, Capabilities: slices.Clone(spec.Capabilities), ProtocolIDs: protocols, CredentialFields: slices.Clone(fields), CheckCapabilities: slices.Clone(spec.CheckCapabilities)})
	}
	return items, nil
}

// Find rejects an unknown ID instead of inventing a display or protocol contract.
func (q *AdapterMetadataQuery) Find(ctx context.Context, actor identityapp.Principal, key string) (AdapterMetadata, error) {
	items, err := q.List(ctx, actor)
	if err != nil {
		return AdapterMetadata{}, err
	}
	for _, item := range items {
		if item.AdapterKey == key {
			return item, nil
		}
	}
	return AdapterMetadata{}, ErrAdapterNotFound
}

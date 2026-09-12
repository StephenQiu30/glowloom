package application

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"

	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

type ReferenceImageCompilerDescriptor struct {
	AdapterRef     domain.GenerationContractRef `json:"adapter_ref"`
	CompilerRef    domain.GenerationContractRef `json:"compiler_ref"`
	CapabilityHash string                       `json:"capability_hash"`
}

type ReferenceImageCompiler interface {
	ReferenceImageDescriptor() (ReferenceImageCompilerDescriptor, error)
	CompileReferenceImages(ReferenceGenerationTarget, agentapp.AcceptedReferenceBrief, domain.ProviderModelProfileVersion) (ReferenceImageCompilation, error)
}

// Body is invocation-scoped material. It must not enter a Snapshot, receipt or log.
type ReferenceImageRequest struct {
	BundleIndex int             `json:"bundle_index"`
	SlotKey     string          `json:"slot_key"`
	ContentHash string          `json:"content_hash"`
	Body        json.RawMessage `json:"-"`
}

type ReferenceImageCompilation struct {
	Descriptor   ReferenceImageCompilerDescriptor `json:"descriptor"`
	ContractID   string                           `json:"contract_id"`
	TargetRef    domain.GenerationRevisionRef     `json:"target_ref"`
	BriefRef     ReferenceBriefRevisionRef        `json:"brief_ref"`
	ProfileRef   domain.GenerationRevisionRef     `json:"profile_ref"`
	Requests     []ReferenceImageRequest          `json:"requests"`
	ManifestHash string                           `json:"manifest_hash"`
}

func (registry *MediaFactoryRegistry) referenceCompiler(provider, modality, adapter string) (ReferenceImageCompiler, ReferenceImageCompilerDescriptor, string, error) {
	factory, err := registry.Resolve(provider, modality, adapter)
	if err != nil {
		return nil, ReferenceImageCompilerDescriptor{}, "", err
	}
	compiler, ok := factory.(ReferenceImageCompiler)
	if !ok {
		return nil, ReferenceImageCompilerDescriptor{}, "", errors.New("Reference image compiler is not registered")
	}
	descriptor, err := compiler.ReferenceImageDescriptor()
	if err != nil {
		return nil, ReferenceImageCompilerDescriptor{}, "", err
	}
	if !descriptor.AdapterRef.Valid() || !descriptor.CompilerRef.Valid() || !intentHashPattern.MatchString(descriptor.CapabilityHash) || descriptor.AdapterRef.ContractID != adapter {
		return nil, ReferenceImageCompilerDescriptor{}, "", errors.New("invalid Reference image compiler descriptor")
	}
	type entry struct {
		Key      string
		Factory  MediaFactoryDescriptor
		Compiler *ReferenceImageCompilerDescriptor
	}
	entries := []entry{}
	for key, item := range registry.entries {
		value := entry{Key: key, Factory: item.descriptor}
		if c, yes := item.factory.(ReferenceImageCompiler); yes {
			d, e := c.ReferenceImageDescriptor()
			if e != nil {
				return nil, ReferenceImageCompilerDescriptor{}, "", e
			}
			value.Compiler = &d
		}
		entries = append(entries, value)
	}
	slices.SortFunc(entries, func(a, b entry) int { return strings.Compare(a.Key, b.Key) })
	raw, err := json.Marshal(entries)
	if err != nil {
		return nil, ReferenceImageCompilerDescriptor{}, "", err
	}
	hash, err := canonical.Hash(raw)
	return compiler, descriptor, hash, err
}

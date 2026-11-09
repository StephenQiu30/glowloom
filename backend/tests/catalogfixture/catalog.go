// Package catalogfixture reads public test catalogs from the canonical SQL seed.
// It deliberately has no independent model or rate list.
package catalogfixture

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

type providerSeed struct {
	Slug     string `json:"slug"`
	Versions []struct {
		ID        string                `json:"id"`
		VersionNo int                   `json:"version_no"`
		Payload   domain.PresetDocument `json:"payload"`
	} `json:"versions"`
}

func seed(name string, output any) {
	_, file, _, _ := runtime.Caller(0)
	data, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "db", "data.sql"))
	if err != nil {
		panic(err)
	}
	expression := regexp.MustCompile(`(?s)\$` + name + `\$\s*(\[.*?\])\s*\$` + name + `\$`)
	match := expression.FindSubmatch(data)
	if len(match) != 2 {
		panic("missing SQL seed " + name)
	}
	if err := json.Unmarshal(match[1], output); err != nil {
		panic(err)
	}
}

// BuiltinPresets reads the latest public provider documents from data.sql.
func BuiltinPresets() []domain.BuiltinPreset {
	var providers []providerSeed
	seed("provider_presets", &providers)
	out := make([]domain.BuiltinPreset, 0, len(providers))
	for _, p := range providers {
		doc := p.Versions[len(p.Versions)-1].Payload
		out = append(out, domain.BuiltinPreset{Slug: p.Slug, Name: doc.Name, Config: doc.Config})
	}
	return out
}

func source(adapter string) (domain.PresetDocument, domain.ModelCandidateCatalog) {
	var providers []providerSeed
	seed("provider_presets", &providers)
	var document domain.PresetDocument
	catalog := domain.ModelCandidateCatalog{Version: "unregistered", Models: []domain.ModelCandidatePreset{}}
	slug := ""
	for _, p := range providers {
		v := p.Versions[len(p.Versions)-1]
		if v.Payload.Config.AdapterKey == adapter {
			document = v.Payload
			catalog.Version = v.ID
			slug = p.Slug
			break
		}
	}
	var entries []struct {
		Slug            string                      `json:"slug"`
		PresetVersionID string                      `json:"preset_version_id"`
		Model           domain.ModelCandidatePreset `json:"model"`
	}
	seed("model_presets", &entries)
	for _, entry := range entries {
		if entry.Slug == slug && entry.PresetVersionID == catalog.Version {
			catalog.Models = append(catalog.Models, entry.Model)
		}
	}
	return document, catalog
}

// BuiltinModelCatalog reads the latest cloud catalog or runtime-only Codex fixture.
func BuiltinModelCatalog(adapter string) domain.ModelCandidateCatalog {
	if adapter == "codex" {
		one := 1
		return domain.ModelCandidateCatalog{Version: "runtime-selection-v1", Models: []domain.ModelCandidatePreset{
			{Key: "codex-chat", AdapterKey: "codex", DisplayName: "Codex 聊天 · 选择节点模型", Capability: "text.generate", Modes: []string{"text_to_text"}, SourceURL: "https://learn.chatgpt.com/docs/app-server", ReviewedAt: "2026-10-20", Lifecycle: "runtime_selection", RequiresModelSelection: true},
			{Key: "codex-image", AdapterKey: "codex", DisplayName: "Codex 生图 · 选择节点模型", Capability: "image.generate", Modes: []string{"text_to_image"}, SourceURL: "https://learn.chatgpt.com/docs/app-server", ReviewedAt: "2026-10-20", Lifecycle: "runtime_selection", RequiresModelSelection: true, MaxOutputs: &one},
		}}
	}
	document, catalog := source(adapter)
	if len(catalog.Models) == 0 {
		return catalog
	}
	return domain.ModelCatalogForPreset(document, catalog)
}

// ModelCatalogForPreset merges explicit templates with the SQL source for tests.
func ModelCatalogForPreset(document domain.PresetDocument) domain.ModelCandidateCatalog {
	_, catalog := source(document.Config.AdapterKey)
	return domain.ModelCatalogForPreset(document, catalog)
}

// ModelCatalogForConfig limits the SQL test catalog to the given configuration.
func ModelCatalogForConfig(config domain.ProviderConfig) domain.ModelCandidateCatalog {
	document, catalog := source(config.AdapterKey)
	document.Config = config
	return domain.ModelCatalogForPreset(document, catalog)
}

// LegacyBuiltinModelCatalog supplies unversioned historical pricing fixtures.
// New SQL candidates require explicit matching algorithms; old snapshots did not.
func LegacyBuiltinModelCatalog(adapter string) domain.ModelCandidateCatalog {
	catalog := BuiltinModelCatalog(adapter)
	for i := range catalog.Models {
		catalog.Models[i].MeteringStrategy = ""
	}
	return catalog
}

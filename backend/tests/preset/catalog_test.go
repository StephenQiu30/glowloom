package preset_test

import (
	"slices"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	presetcatalog "github.com/StephenQiu30/lanverse/backend/internal/preset/catalog"
)

func TestCuratedCatalogPublishesFourTraceablePresetReleases(t *testing.T) {
	releases, err := presetcatalog.CuratedReleases()
	if err != nil {
		t.Fatal(err)
	}
	if len(releases) != 4 {
		t.Fatalf("expected four curated Preset releases, got %d", len(releases))
	}

	wantKeys := []string{
		"chinese-fantasy-animation",
		"cyberpunk-animation",
		"period-cinematic-realism",
		"urban-cinematic-realism",
	}
	keys := make([]string, len(releases))
	seenHashes := make(map[string]struct{}, len(releases))
	wantHashes := map[string]string{
		"chinese-fantasy-animation": "1ce0d5bef265a2ca9a47ed8e72610d34b94bc1abbeb0b2cb956dbc8f5e7f0fe1",
		"cyberpunk-animation":       "4c8fa17d000ede0f3c3ffaa9acba460444c605996bf50e1da321dde0769a339a",
		"period-cinematic-realism":  "b1251061a4f9dd46db77fae8e055dd1225dcc0db12cc550f6022e20c6b02dff0",
		"urban-cinematic-realism":   "178ac3194a4dc91f11cea4131a444e28a5e2f3a2320bb22516116a3ee2fd4666",
	}
	for index, release := range releases {
		keys[index] = release.Key
		if release.Release != "2026.09.13" || release.DefaultMode != "faithful" {
			t.Fatalf("Preset release is not a fixed faithful release: %#v", release)
		}
		if release.Provenance.Origin != "first_party" || release.Provenance.LicenseSPDX != "MIT" ||
			release.Provenance.NoticePath != "backend/internal/preset/catalog/NOTICE.md" {
			t.Fatalf("Preset release provenance is not traceable: %#v", release.Provenance)
		}
		if len(release.CapabilityManifest) != 6 || len(release.PurposeProfiles) != 6 {
			t.Fatalf("Preset release does not cover all six target purposes: %#v", release)
		}
		if len(release.WorldAdaptationRules) == 0 {
			t.Fatalf("Preset release lacks explicit world-adaptation decisions: %#v", release)
		}
		if _, exists := seenHashes[release.ContentHash]; exists {
			t.Fatalf("Preset releases share a content identity: %s", release.ContentHash)
		}
		if release.ContentHash != wantHashes[release.Key] {
			t.Fatalf("Preset release identity drifted: %#v", release)
		}
		seenHashes[release.ContentHash] = struct{}{}
	}
	if !slices.Equal(keys, wantKeys) {
		t.Fatalf("unexpected curated Preset inventory: got=%v want=%v", keys, wantKeys)
	}
}

func TestCuratedCatalogBindsRealSkillAndPolicyContent(t *testing.T) {
	releases, err := presetcatalog.CuratedReleases()
	if err != nil {
		t.Fatal(err)
	}
	qcPolicy, err := presetcatalog.ReferenceQCPolicy()
	if err != nil {
		t.Fatal(err)
	}
	modelPolicy, err := presetcatalog.VisualModelCapabilityPolicy()
	if err != nil {
		t.Fatal(err)
	}
	if qcPolicy.ContentHash != "cd2edeb49a2e29803a62ce395f8ce7b5f3ff45f69706bb529278a2a648573aec" ||
		modelPolicy.ContentHash != "145b1cf19e5f6fdd2ac676b98f1d66d55a3867a5d3ac1d041d44ea0009c308a4" {
		t.Fatalf("curated Preset policy identity drifted: qc=%s model=%s", qcPolicy.ContentHash, modelPolicy.ContentHash)
	}

	for _, release := range releases {
		if len(release.SkillReleaseRefs) != 1 ||
			release.SkillReleaseRefs[0].Owner != "agent/skill" ||
			release.SkillReleaseRefs[0].Key != "build-storygraph" ||
			release.SkillReleaseRefs[0].ContentHash != contract.StoryGraphSkillBundleHash {
			t.Fatalf("Preset release does not bind the installed Skill content: %#v", release.SkillReleaseRefs)
		}
		if release.QCPolicyRef.Owner != "preset/policy" || release.QCPolicyRef.Key != qcPolicy.Key ||
			release.QCPolicyRef.ContentHash != qcPolicy.ContentHash {
			t.Fatalf("Preset release does not bind the curated QC policy: %#v", release.QCPolicyRef)
		}
		if release.ModelCapabilityPolicyRef.Owner != "preset/policy" ||
			release.ModelCapabilityPolicyRef.Key != modelPolicy.Key ||
			release.ModelCapabilityPolicyRef.ContentHash != modelPolicy.ContentHash {
			t.Fatalf("Preset release does not bind the visual model policy: %#v", release.ModelCapabilityPolicyRef)
		}
	}
}

func TestCuratedCatalogLookupRequiresExactImmutableIdentity(t *testing.T) {
	release, found, err := presetcatalog.FindCuratedRelease("urban-cinematic-realism", "2026.09.13")
	if err != nil || !found || release.Key != "urban-cinematic-realism" {
		t.Fatalf("find exact curated release: release=%#v found=%t err=%v", release, found, err)
	}
	if _, found, err = presetcatalog.FindCuratedRelease("urban-cinematic-realism", "current"); err != nil || found {
		t.Fatalf("mutable Preset alias resolved: found=%t err=%v", found, err)
	}
	if _, found, err = presetcatalog.FindCuratedRelease("unknown", "2026.09.13"); err != nil || found {
		t.Fatalf("unknown Preset resolved: found=%t err=%v", found, err)
	}
}

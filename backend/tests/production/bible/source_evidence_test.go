package bible_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	bible "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

func TestSourceEvidenceManifestUsesDeterministicUnicodeCoverageAndSemanticBoundaries(t *testing.T) {
	text := "第1集\n林一😀走进大厅。\n\n场景二\n钟声响起。\n\n第二集\n林一回头。\n\n第12集\n终章。"
	input := bible.SourceEvidenceManifestInput{
		ManifestID:         "60000000-0000-0000-0000-000000000001",
		WorkspaceID:        "30000000-0000-0000-0000-000000000001",
		WorkflowRunID:      "70000000-0000-0000-0000-000000000001",
		NodeRunID:          "80000000-0000-0000-0000-000000000001",
		RootInputHash:      strings.Repeat("a", 64),
		NormalizedText:     text,
		MaxShardCodePoints: 24,
		OverlapCodePoints:  3,
	}

	first, err := bible.BuildSourceEvidenceManifest(input)
	if err != nil {
		t.Fatalf("build source Evidence manifest: %v", err)
	}
	input.ManifestID = "60000000-0000-0000-0000-000000000099"
	second, err := bible.BuildSourceEvidenceManifest(input)
	if err != nil {
		t.Fatalf("rebuild source Evidence manifest: %v", err)
	}
	if first.ManifestHash != second.ManifestHash || first.CoverageHash != second.CoverageHash {
		t.Fatalf("random manifest ID changed deterministic hashes: first=%#v second=%#v", first, second)
	}
	if first.Version != 1 || first.Stage != "extract_source_evidence" || len(first.Shards) < 2 {
		t.Fatalf("unexpected initial manifest: %#v", first)
	}

	textRunes := []rune(text)
	position := 0
	foundArabicMarker, foundChineseMarker := false, false
	for _, shard := range first.Shards {
		if shard.Status != "active" || shard.Kind != "source_slice" || shard.LogicalStart != position ||
			shard.LogicalEnd <= shard.LogicalStart || shard.LogicalEnd > len(textRunes) ||
			shard.ContextStart > shard.LogicalStart || shard.ContextEnd < shard.LogicalEnd ||
			shard.ContextStart < 0 || shard.ContextEnd > len(textRunes) || len(shard.SourceHashes) != 1 {
			t.Fatalf("invalid source shard: %#v", shard)
		}
		if shard.LogicalStart > 0 && shard.ContextStart == shard.LogicalStart {
			t.Fatalf("non-leading shard did not record left overlap: %#v", shard)
		}
		for _, marker := range shard.EpisodeMarkerHints {
			foundArabicMarker = foundArabicMarker || marker.Label == "第12集"
			foundChineseMarker = foundChineseMarker || marker.Label == "第二集"
		}
		position = shard.LogicalEnd
	}
	if position != len(textRunes) || !foundArabicMarker || !foundChineseMarker {
		t.Fatalf("coverage=%d/%d markers Arabic=%t Chinese=%t", position, len(textRunes), foundArabicMarker, foundChineseMarker)
	}
	if err = bible.ValidateSourceEvidenceManifest(first, text); err != nil {
		t.Fatalf("validate source Evidence manifest: %v", err)
	}
}

func TestSourceEvidenceReshardSupersedesParentWithoutCoverageLoss(t *testing.T) {
	text := "第一集\n甲。\n\n第二场\n乙。\n\n第二集\n丙。\n"
	manifest, err := bible.BuildSourceEvidenceManifest(bible.SourceEvidenceManifestInput{
		ManifestID:         "60000000-0000-0000-0000-000000000001",
		WorkspaceID:        "30000000-0000-0000-0000-000000000001",
		WorkflowRunID:      "70000000-0000-0000-0000-000000000001",
		NodeRunID:          "80000000-0000-0000-0000-000000000001",
		RootInputHash:      strings.Repeat("a", 64),
		NormalizedText:     text,
		MaxShardCodePoints: 200,
		OverlapCodePoints:  2,
	})
	if err != nil || len(manifest.Shards) != 1 {
		t.Fatalf("build single shard manifest: %#v err=%v", manifest, err)
	}
	parent := manifest.Shards[0]
	resharded, err := bible.ReshardSourceEvidenceManifest(manifest, parent.Key, text, 12, 2)
	if err != nil {
		t.Fatalf("reshard source Evidence manifest: %v", err)
	}
	if resharedParent := resharedShard(t, resharded.Shards, parent.Key); resharedParent.Status != "superseded" {
		t.Fatalf("parent shard was not superseded: %#v", resharedParent)
	}
	if resharded.Version != 2 || resharded.ParentManifestHash == nil || *resharded.ParentManifestHash != manifest.ManifestHash ||
		resharded.ManifestHash == manifest.ManifestHash || resharded.CoverageHash != manifest.CoverageHash {
		t.Fatalf("unexpected reshard identity: before=%#v after=%#v", manifest, resharded)
	}
	active := activeShards(resharded.Shards)
	if len(active) < 2 || active[0].LogicalStart != parent.LogicalStart || active[len(active)-1].LogicalEnd != parent.LogicalEnd {
		t.Fatalf("children do not cover the parent: %#v", active)
	}
	for index := 1; index < len(active); index++ {
		if active[index-1].LogicalEnd != active[index].LogicalStart || active[index].ParentKey != parent.Key {
			t.Fatalf("children contain a gap or invalid parent: %#v", active)
		}
	}
	if err = bible.ValidateSourceEvidenceManifest(resharded, text); err != nil {
		t.Fatalf("validate reshared manifest: %v", err)
	}
}

func TestSourceEvidenceCandidateCorrectsLocalOffsetsAndRejectsFabricatedEvidence(t *testing.T) {
	text := "第一集\n甲说你好😀。\n\n第二集\n乙回答。"
	anchor := "乙回答"
	absoluteStart := slices.Index([]rune(text), []rune(anchor)[0])
	for absoluteStart >= 0 && string([]rune(text)[absoluteStart:absoluteStart+len([]rune(anchor))]) != anchor {
		next := slices.Index([]rune(text)[absoluteStart+1:], []rune(anchor)[0])
		if next < 0 {
			break
		}
		absoluteStart += next + 1
	}
	shard := bible.SourceEvidenceShard{
		Key: "source:00000018:00000027", TreePath: "0001", Kind: "source_slice", Status: "active",
		LogicalStart: absoluteStart, LogicalEnd: len([]rune(text)), ContextStart: absoluteStart - 2,
		ContextEnd: len([]rune(text)), SourceHashes: []string{strings.Repeat("b", 64)},
	}
	localStart := absoluteStart - shard.ContextStart
	evidenceValue := map[string]any{
		"source_start": localStart, "source_end": localStart + len([]rune(anchor)),
		"text_hash": bible.SourceTextHash(anchor), "exact_anchor": anchor, "episode_number": 2,
	}
	raw, err := json.Marshal(map[string]any{
		"observations": []any{map[string]any{
			"observation_key": "observation:reply", "kind": "event", "proposed_key": "event:reply",
			"label": anchor, "facts": []string{"乙作出回答"}, "ambiguities": []string{},
			"evidence": []any{evidenceValue, evidenceValue},
		}},
		"review_issues": []any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := bible.DecodeAndNormalizeSourceEvidenceCandidate(raw, text, shard)
	if err != nil {
		t.Fatalf("normalize chunk-local Evidence: %v", err)
	}
	if len(candidate.Observations[0].Evidence) != 1 {
		t.Fatalf("duplicate range+hash Evidence was not collapsed: %#v", candidate.Observations[0].Evidence)
	}
	evidence := candidate.Observations[0].Evidence[0]
	if evidence.SourceStart != absoluteStart || evidence.SourceEnd != absoluteStart+len([]rune(anchor)) || evidence.ExactAnchor != anchor {
		t.Fatalf("local Evidence was not corrected to absolute code points: %#v", evidence)
	}

	forged := strings.ReplaceAll(string(raw), anchor, "原稿没有的回答")
	if _, err = bible.DecodeAndNormalizeSourceEvidenceCandidate([]byte(forged), text, shard); err == nil {
		t.Fatal("fabricated Evidence was accepted")
	}
}

func activeShards(values []bible.SourceEvidenceShard) []bible.SourceEvidenceShard {
	result := make([]bible.SourceEvidenceShard, 0, len(values))
	for _, value := range values {
		if value.Status == "active" {
			result = append(result, value)
		}
	}
	slices.SortFunc(result, func(left, right bible.SourceEvidenceShard) int { return left.LogicalStart - right.LogicalStart })
	return result
}

func resharedShard(t *testing.T, values []bible.SourceEvidenceShard, key string) bible.SourceEvidenceShard {
	t.Helper()
	for _, value := range values {
		if value.Key == key {
			return value
		}
	}
	t.Fatalf("shard %q not found", key)
	return bible.SourceEvidenceShard{}
}

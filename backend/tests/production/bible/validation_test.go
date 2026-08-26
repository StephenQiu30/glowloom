package bible_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

func TestDecodeAndValidateCandidateBindsEvidenceToSource(t *testing.T) {
	anchor := "小兰"
	hash := sha256.Sum256([]byte(anchor))
	raw, err := json.Marshal(map[string]any{
		"entities": []any{map[string]any{
			"entity_key": "character.xiaolan", "kind": "character", "canonical_name": anchor, "normalized_name": anchor,
			"aliases": []any{}, "stable_spec": map[string]any{}, "episode_numbers": []any{1},
			"evidence": []any{map[string]any{"source_start": 0, "source_end": 2, "text_hash": hex.EncodeToString(hash[:]), "exact_anchor": anchor, "episode_number": 1}},
			"states":   []any{}, "ambiguities": []any{},
		}},
		"world_entries": []any{}, "review_issues": []any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = bibledomain.DecodeAndValidateCandidate(raw, "小兰走进房间"); err != nil {
		t.Fatalf("expected candidate to pass: %v", err)
	}

	if _, err = bibledomain.DecodeAndValidateCandidate(raw, "小明走进房间"); err == nil {
		t.Fatal("expected mismatched exact source anchor to fail")
	}
}

func TestDecodeAndValidateCandidateRejectsUnknownWorldReference(t *testing.T) {
	anchor := "规则"
	hash := sha256.Sum256([]byte(anchor))
	raw, err := json.Marshal(map[string]any{
		"entities": []any{},
		"world_entries": []any{map[string]any{
			"entry_key": "world.rule", "category": "规则", "title": "规则", "facts": []any{"事实"}, "rules": []any{},
			"entity_keys": []any{"character.missing"}, "episode_numbers": []any{1},
			"evidence": []any{map[string]any{"source_start": 0, "source_end": 2, "text_hash": hex.EncodeToString(hash[:]), "exact_anchor": anchor}}, "ambiguities": []any{},
		}},
		"review_issues": []any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = bibledomain.DecodeAndValidateCandidate(raw, anchor); err == nil {
		t.Fatal("expected unknown entity reference to fail")
	}
}

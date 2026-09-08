package agent_test

import (
	"testing"

	contract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

func TestTextEvidenceIndexPreservesUnicodeAndOverlappingOccurrences(t *testing.T) {
	index := contract.NewTextEvidenceIndex(contract.TextSource{Text: "甲😀e\u0301\u2028aaaa\n尾声"})
	for _, expected := range []struct {
		evidence   contract.TextEvidence
		start, end int
	}{
		{contract.TextEvidence{Block: 0, Quote: "😀e\u0301"}, 1, 4},
		{contract.TextEvidence{Block: 1, Quote: "aa", Occurrence: new(2)}, 7, 9},
		{contract.TextEvidence{Block: 2, Quote: "尾声"}, 10, 12},
	} {
		start, end, err := index.Resolve(expected.evidence)
		if err != nil || start != expected.start || end != expected.end {
			t.Fatalf("Unicode evidence changed: %d:%d %v", start, end, err)
		}
	}
	if _, _, err := index.Resolve(contract.TextEvidence{Block: 1, Quote: "aa"}); err == nil {
		t.Fatal("ambiguous overlapping quote accepted")
	}
	if _, _, err := index.Resolve(contract.TextEvidence{Block: 1, Quote: "aa", Occurrence: new(3)}); err == nil {
		t.Fatal("nonexistent occurrence accepted")
	}
}

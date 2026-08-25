package application

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/domain"
)

func TestBuildPackageIsDeterministicAndContainsRequiredFiles(t *testing.T) {
	shots := []domain.Shot{{ID: "00000000-0000-0000-0000-000000000001", EpisodeID: "00000000-0000-0000-0000-000000000002", Position: 1, Title: "开场", ContentHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Spec: map[string]any{"duration_ms": float64(1800)}, NarrativeUnitIDs: []string{"00000000-0000-0000-0000-000000000003"}}}
	first, err := buildPackage(shots[0].EpisodeID, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", shots)
	if err != nil {
		t.Fatal(err)
	}
	second, err := buildPackage(shots[0].EpisodeID, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", shots)
	if err != nil {
		t.Fatal(err)
	}
	if first.ContentHash != second.ContentHash || !bytes.Equal(first.Bytes, second.Bytes) {
		t.Fatal("fixed input did not produce an identical package")
	}
	reader, err := zip.NewReader(bytes.NewReader(first.Bytes), int64(len(first.Bytes)))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, file := range reader.File {
		names[file.Name] = true
	}
	for _, name := range []string{"manifest.json", "storyboard.json", "storyboard.csv", "storyboard.html"} {
		if !names[name] {
			t.Fatalf("package is missing %s", name)
		}
	}
}

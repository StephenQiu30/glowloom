package otvectors_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

func TestSequenceProtocolVectors(t *testing.T) {
	var vectors []struct {
		Name   string
		Before []domain.Atom
		A, B   domain.SequenceEdit
	}
	for _, file := range []string{"../testdata/sequence.json", "../testdata/sequence-random.json"} {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var part []struct {
			Name   string
			Before []domain.Atom
			A, B   domain.SequenceEdit
		}
		if err := json.Unmarshal(data, &part); err != nil {
			t.Fatal(err)
		}
		vectors = append(vectors, part...)
	}
	type result struct {
		Name string              `json:"name"`
		AP   domain.SequenceEdit `json:"ap"`
		BP   domain.SequenceEdit `json:"bp"`
	}
	results := make([]result, 0, len(vectors))
	for _, v := range vectors {
		a, b, err := domain.TransformSequencePair(v.A, v.B, domain.SequenceLength(v.Before))
		if err != nil {
			t.Fatal(v.Name, err)
		}
		results = append(results, result{v.Name, a, b})
	}
	if path := os.Getenv("LV_OT_VECTOR_OUTPUT"); path != "" {
		data, err := json.Marshal(results)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

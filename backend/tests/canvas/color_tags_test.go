package canvas_test

import (
	"encoding/json"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

func TestCanvasColorTags(t *testing.T) {
	for _, tc := range []struct {
		value string
		valid bool
	}{
		{`[]`, true}, {`["cyan","blue","purple","orange","yellow"]`, true},
		{`["yellow","cyan"]`, true}, {`["cyan","cyan"]`, false},
		{`null`, false}, {`"cyan"`, false}, {`["red"]`, false},
		{`[null]`, false}, {`["default"]`, false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			doc := otDoc()
			doc.Nodes[0].Config.Editor = &domain.EditorConfig{SchemaVersion: 1, Metadata: map[string]json.RawMessage{"colorTags": raw(tc.value), "textBackground": raw(`"orange"`)}}
			err := domain.ValidateGraph(doc)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
			if !tc.valid {
				return
			}
			body, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			var reopened domain.Document
			if err := json.Unmarshal(body, &reopened); err != nil {
				t.Fatal(err)
			}
			if string(reopened.Nodes[0].Config.Editor.Metadata["colorTags"]) != tc.value || string(reopened.Nodes[0].Config.Editor.Metadata["textBackground"]) != `"orange"` {
				t.Fatal("tags or background changed on reopen")
			}
		})
	}
}

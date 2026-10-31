package canvas_test

import (
	"encoding/json"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

func TestCanvasTextAlignment(t *testing.T) {
	for _, tc := range []struct {
		name, node, alignment string
		valid                 bool
	}{
		{"left", "paragraph", `"left"`, true}, {"center", "paragraph", `"center"`, true}, {"right heading", "heading", `"right"`, true},
		{"justify", "paragraph", `"justify"`, false}, {"CSS", "paragraph", `"url(x)"`, false}, {"number", "paragraph", `1`, false}, {"null", "paragraph", `null`, false}, {"wrong node", "blockquote", `"left"`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := otDoc()
			rich := `{"schemaVersion":1,"content":{"type":"doc","content":[{"type":"` + tc.node + `","attrs":{"textAlign":` + tc.alignment + `},"content":[{"type":"text","text":"对齐"}]}]}}`
			if err := json.Unmarshal(raw(`{"format":"tiptap","format_version":1,"composer":`+textComposerJSON+`,"rich_text":`+rich+`}`), &doc.Nodes[0].Config); err != nil {
				t.Fatal(err)
			}
			err := domain.ValidateGraph(doc)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
		})
	}
}

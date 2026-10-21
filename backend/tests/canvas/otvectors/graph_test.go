package otvectors_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

func writeOracle(t *testing.T, value any) {
	t.Helper()
	if path := os.Getenv("GLOWLOOM_OT_VECTOR_OUTPUT"); path != "" {
		body, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
func TestFieldPolicyProtocolVectors(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "../../../internal/canvas/domain/operation_fields.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]map[string]any{}
	ast.Inspect(file, func(node ast.Node) bool {
		v, ok := node.(*ast.ValueSpec)
		if !ok || len(v.Names) != 1 || v.Names[0].Name != "operationFields" {
			return true
		}
		for _, elt := range v.Values[0].(*ast.CompositeLit).Elts {
			entry := elt.(*ast.KeyValueExpr)
			key, _ := strconv.Unquote(entry.Key.(*ast.BasicLit).Value)
			rule := map[string]any{}
			for _, part := range entry.Value.(*ast.CompositeLit).Elts {
				kv := part.(*ast.KeyValueExpr)
				name := kv.Key.(*ast.Ident).Name
				switch x := kv.Value.(type) {
				case *ast.BasicLit:
					rule[name], _ = strconv.Unquote(x.Value)
				case *ast.Ident:
					rule[name] = x.Name == "true"
				case *ast.CompositeLit:
					values := []string{}
					for _, value := range x.Elts {
						s, _ := strconv.Unquote(value.(*ast.BasicLit).Value)
						values = append(values, s)
					}
					rule[name] = values
				}
			}
			fields[key] = rule
		}
		return false
	})
	if len(fields) < 100 {
		t.Fatal("incomplete policy oracle", len(fields))
	}
	writeOracle(t, fields)
}
func TestGraphProtocolVectors(t *testing.T) {
	type vector struct {
		Name   string
		Before []domain.Atom
		A, B   domain.SequenceEdit
	}
	vectors := []vector{}
	for _, file := range []string{"../testdata/sequence.json", "../testdata/sequence-random.json"} {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var part []vector
		if err := json.Unmarshal(body, &part); err != nil {
			t.Fatal(err)
		}
		vectors = append(vectors, part...)
	}
	type result struct {
		Name  string          `json:"name"`
		Base  domain.Document `json:"base"`
		A, B  []domain.Operation
		AP    []domain.Operation `json:"ap"`
		BP    []domain.Operation `json:"bp"`
		Nodes []domain.Node      `json:"nodes"`
	}
	results := []result{}
	for _, v := range vectors {
		var d domain.Document
		const template = `{"id":"11111111-1111-4111-8111-111111111111","project_id":"22222222-2222-4222-8222-222222222222","revision":1,"nodes":[{"id":"33333333-3333-4333-8333-333333333333","title":"N","node_type":"image_generation","node_action":"generate","x":0,"y":0,"z_index":0,"input_revision":1,"config":{"draft":{"prompt":"","negative_prompt":"","aspect_ratio":"","candidate_count":0,"composer":{"schemaVersion":1,"document":{"type":"doc","content":[{"type":"paragraph"}]},"references":[{"id":"r","source":{"kind":"media","sourceId":"44444444-4444-4444-8444-444444444444","sourceVersion":"1"},"bindingGeneration":1,"mediaKind":"image","role":"reference_image","order":0},{"id":"ref","source":{"kind":"media","sourceId":"55555555-5555-4555-8555-555555555555","sourceVersion":"1"},"bindingGeneration":1,"mediaKind":"image","role":"reference_image","order":1}],"params":{},"modelVersion":null,"mode":null,"labelState":{"highWater":{"image":2,"video":0,"audio":0,"subject":0},"allocations":{"r":{"labelKind":"image","ordinal":1},"ref":{"labelKind":"image","ordinal":2}}}}}}}],"edges":[]}`
		if err := json.Unmarshal([]byte(template), &d); err != nil {
			t.Fatal(err)
		}
		document, err := domain.SequencePrompt(v.Before)
		if err != nil {
			t.Fatal(err)
		}
		d.Nodes[0].Config.Draft.Composer.Document = document
		id := d.Nodes[0].ID
		path := []string{"config", "draft", "composer", "document"}
		a := []domain.Operation{{Type: "text", NodeID: id, Path: path, Text: v.A}, {Type: "set", NodeID: id, Path: []string{"position"}, Value: json.RawMessage(`{"x":10,"y":20}`)}}
		b := []domain.Operation{{Type: "text", NodeID: id, Path: path, Text: v.B}, {Type: "set", NodeID: id, Path: []string{"title"}, Value: json.RawMessage(`"later"`)}, {Type: "set", NodeID: id, Path: []string{"position"}, Value: json.RawMessage(`{"x":30,"y":40}`)}}
		ap, bp, err := domain.TransformOperationPair(d, a, b)
		if err != nil {
			t.Fatal(v.Name, err)
		}
		first, err := domain.ApplyOperations(d, a)
		if err != nil {
			t.Fatal(err)
		}
		last, err := domain.ApplyOperations(first.Document, bp)
		if err != nil {
			t.Fatal(v.Name, err)
		}
		results = append(results, result{v.Name, d, a, b, ap, bp, last.Document.Nodes})
	}
	writeOracle(t, results)
}

package audit_test

import (
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"

	auditapp "github.com/StephenQiu30/glowloom-server/backend/internal/audit/application"
)

// The catalog's shared mutation helper emits an empty audit summary. Extract its
// actual action arguments so a new producer cannot silently outrun the consumer.
func TestCatalogMutationAuditProducerConsumerContract(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate source contract test")
	}
	files, err := filepath.Glob(filepath.Join(filepath.Dir(source), "..", "..", "internal", "catalog", "adapter", "postgres", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	actions := map[string]bool{}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(source, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			functionExpr := call.Fun
			switch indexed := functionExpr.(type) {
			case *ast.IndexExpr:
				functionExpr = indexed.X
			case *ast.IndexListExpr:
				functionExpr = indexed.X
			}
			function, ok := functionExpr.(*ast.Ident)
			if !ok || function.Name != "catalogMutation" {
				return true
			}
			if len(call.Args) < 5 {
				t.Errorf("%s: invalid catalogMutation argument contract", name)
				return false
			}
			literal, ok := call.Args[4].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				t.Errorf("%s: catalogMutation action must be a reviewable string literal", name)
				return false
			}
			action, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Error(err)
				return false
			}
			actions[action] = true
			return true
		})
	}
	if len(actions) == 0 {
		t.Fatal("no catalogMutation producer actions discovered")
	}
	ordered := make([]string, 0, len(actions))
	for action := range actions {
		ordered = append(ordered, action)
	}
	sort.Strings(ordered)
	t.Logf("checked %d actual catalogMutation producer actions", len(ordered))
	consumer := auditapp.NewRecordedActionParser()
	for _, action := range ordered {
		t.Run(action, func(t *testing.T) {
			record := providerCatalogRecord(t, action, uuid.New())
			parsed, err := consumer.Parse(record)
			if err != nil {
				t.Fatal("actual catalog producer action rejected", err)
			}
			if parsed.Action != action || len(parsed.Before) != 0 || len(parsed.After) != 0 {
				t.Fatal("catalog audit wire contract changed", parsed)
			}
			var body map[string]any
			if err := json.Unmarshal(record.Value, &body); err != nil {
				t.Fatal(err)
			}
			data := body["data"].(map[string]any)
			for _, side := range []string{"before", "after"} {
				for _, field := range []string{"config", "credential", "api_key", "signed_url", "name", "note"} {
					data[side] = map[string]any{field: "private"}
					record.Value, _ = json.Marshal(body)
					if _, err := consumer.Parse(record); !errors.Is(err, auditapp.ErrInvalidEvent) {
						t.Fatalf("unreviewed %s.%s accepted: %v", side, field, err)
					}
				}
				delete(data, side)
			}
			data["action"] = action + ".unregistered"
			record.Value, _ = json.Marshal(body)
			if _, err := consumer.Parse(record); !errors.Is(err, auditapp.ErrInvalidEvent) {
				t.Fatal("unknown action accepted", err)
			}
		})
	}
}

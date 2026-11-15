package gateway_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net/http"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// Exercise the actual registered contracts, rather than copying the gateway's
// table into another expected table. This catches a missing route when an owner
// adds an endpoint, including nested RouterGroups and non-owner legacy wiring.
func TestDomainRoutesCoverRegisteredPublicContracts(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "../.."))
	server, _ := domainProxy(t, func(owner string, w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Owner", owner)
		w.WriteHeader(204)
	})
	type contract struct{ method, path, owner string }
	var contracts []contract
	for area, owner := range map[string]string{
		"identity/adapter/http": "identity", "creation": "creation", "media": "media", "execution": "execution",
		"internal/agent/adapter/http": "creation", "internal/analytics/adapter/http": "execution", "internal/observability/adapter/http": "identity",
	} {
		err := filepath.WalkDir(filepath.Join(root, area), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			if !strings.Contains(path, "/adapter/http/") && !strings.Contains(path, "/adapter/websocket/") {
				return nil
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return err
			}
			for _, declaration := range file.Decls {
				fn, ok := declaration.(*ast.FuncDecl)
				if !ok || fn.Body == nil || !strings.HasPrefix(fn.Name.Name, "Register") {
					continue
				}
				if area == "internal/agent/adapter/http" && fn.Name.Name != "Register" && fn.Name.Name != "RegisterSkillManagement" {
					continue
				}
				if area == "internal/agent/adapter/http" && filepath.Base(path) != "public.go" && filepath.Base(path) != "skills.go" {
					continue
				}
				groups := make(map[string]string)
				ast.Inspect(fn.Body, func(node ast.Node) bool {
					if assign, ok := node.(*ast.AssignStmt); ok && len(assign.Lhs) == 1 && len(assign.Rhs) == 1 {
						id, isID := assign.Lhs[0].(*ast.Ident)
						call, isCall := assign.Rhs[0].(*ast.CallExpr)
						if isID && isCall {
							if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Group" {
								groups[id.Name] = groupPrefix(call, groups)
							}
						}
					}
					call, ok := node.(*ast.CallExpr)
					if !ok || len(call.Args) == 0 {
						return true
					}
					sel, ok := call.Fun.(*ast.SelectorExpr)
					if !ok {
						return true
					}
					switch sel.Sel.Name {
					case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
					default:
						return true
					}
					route, ok := stringLiteral(call.Args[0])
					if !ok {
						t.Errorf("nonliteral public route in %s", path)
						return true
					}
					prefix := groupPrefix(sel.X, groups)
					if strings.HasSuffix(path, "/execution/billing/adapter/http/credit_commands.go") {
						prefix = "/platform"
					}
					full := prefix + route
					if full == "" {
						t.Errorf("empty public route in %s", path)
						return true
					}
					actualOwner := owner
					if owner == "identity" && (strings.HasPrefix(full, "/teams/") || strings.HasPrefix(full, "/platform/teams/")) {
						actualOwner = "creation"
					}
					contracts = append(contracts, contract{sel.Sel.Name, "/api" + full, actualOwner})
					return true
				})
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(contracts) < 350 {
		t.Fatalf("incomplete registration inventory: %d", len(contracts))
	}
	for _, route := range contracts {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			parts := strings.Split(route.path, "/")
			for i, part := range parts {
				if strings.HasPrefix(part, ":") {
					parts[i] = "fixture"
				}
			}
			req, _ := http.NewRequestWithContext(t.Context(), route.method, server.URL+strings.Join(parts, "/"), nil)
			res, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = res.Body.Close()
			if res.StatusCode != 204 || res.Header.Get("Owner") != route.owner {
				t.Fatalf("registered contract %s %s: status=%d owner=%q want=%q", route.method, route.path, res.StatusCode, res.Header.Get("Owner"), route.owner)
			}
		})
	}
	t.Logf("verified %d registered method/path contracts over TCP", len(contracts))
}

func groupPrefix(expr ast.Expr, groups map[string]string) string {
	switch node := expr.(type) {
	case *ast.Ident:
		return groups[node.Name]
	case *ast.CallExpr:
		if sel, ok := node.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Group" && len(node.Args) > 0 {
			prefix, _ := stringLiteral(node.Args[0])
			return groupPrefix(sel.X, groups) + prefix
		}
	}
	return ""
}

func stringLiteral(expr ast.Expr) (string, bool) {
	value, ok := expr.(*ast.BasicLit)
	if !ok || value.Kind != token.STRING {
		return "", false
	}
	text, err := strconv.Unquote(value.Value)
	return text, err == nil
}

// Command genapi extracts the HTTP API surface from handlers.go and emits an
// OpenAPI 3.1 document. It is read-only: it never rewrites source, only
// generates docs/api/openapi.yaml (or diffs it in --check mode).
//
// Design note: Go 1.22 http.ServeMux has no runtime pattern enumeration, so we
// statically parse handlers.go for mux.HandleFunc("METHOD /path", handler)
// calls. Only /api/ paths are part of the contract; static assets, /proxy/,
// /v2/ legacy 404s and agent downloads are excluded.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type route struct {
	Method  string
	Path    string
	Handler string
}

func main() {
	check := flag.Bool("check", false, "fail if generated doc differs from committed one")
	root := flag.String("root", ".", "repository root containing cmd/server/handlers.go")
	flag.Parse()

	routes, err := extractRoutes(filepath.Join(*root, "cmd/server/handlers.go"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "genapi: extract routes: %v\n", err)
		os.Exit(1)
	}

	spec, err := buildSpec(routes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "genapi: build spec: %v\n", err)
		os.Exit(1)
	}

	outPath := filepath.Join(*root, "docs/api/openapi.yaml")
	if *check {
		existing, err := os.ReadFile(outPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "genapi: --check: %v (run `go run ./cmd/server/genapi` first)\n", err)
			os.Exit(1)
		}
		if !bytes.Equal(bytes.TrimSpace(existing), bytes.TrimSpace(spec)) {
			fmt.Fprintf(os.Stderr, "genapi: --check: docs/api/openapi.yaml is out of date. Run `go run ./cmd/server/genapi` and commit the diff.\n")
			os.Exit(1)
		}
		fmt.Printf("genapi: ok — %d API routes match docs/api/openapi.yaml\n", len(routes))
		return
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "genapi: mkdir: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(outPath, spec, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "genapi: write: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("genapi: wrote %d routes to %s\n", len(routes), outPath)
}

// extractRoutes parses handlers.go and returns the mux.HandleFunc registrations.
func extractRoutes(path string) ([]route, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, err
	}

	var routes []route
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil {
			continue
		}
		// Only scan the Routes method body.
		if fn.Name.Name != "Routes" || fn.Recv == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel == nil || sel.Sel.Name != "HandleFunc" {
				return true
			}
			// mux.HandleFunc("METHOD /path", s.handler) — first arg is a string literal.
			if len(call.Args) < 2 {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			pattern, err := strconvUnquote(lit.Value)
			if err != nil {
				return true
			}
			method, path, ok := splitPattern(pattern)
			if !ok {
				return true
			}
			// Only the public /api surface is part of the contract.
			if !strings.HasPrefix(path, "/api/") {
				return true
			}
			handler := exprString(call.Args[1])
			routes = append(routes, route{Method: method, Path: path, Handler: handler})
			return true
		})
	}
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Path != routes[j].Path {
			return routes[i].Path < routes[j].Path
		}
		return routes[i].Method < routes[j].Method
	})
	return routes, nil
}

func splitPattern(pattern string) (method, path string, ok bool) {
	parts := strings.SplitN(pattern, " ", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	method = strings.ToUpper(strings.TrimSpace(parts[0]))
	path = strings.TrimSpace(parts[1])
	if method == "" || path == "" || !strings.HasPrefix(path, "/") {
		return "", "", false
	}
	return method, path, true
}

func strconvUnquote(s string) (string, error) {
	if len(s) >= 2 {
		if s[0] == '"' && s[len(s)-1] == '"' {
			return s[1 : len(s)-1], nil
		}
	}
	return s, nil
}

func exprString(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return exprString(v.X) + "." + v.Sel.Name
	case *ast.FuncLit:
		return "(inline func)"
	case *ast.IndexExpr:
		return exprString(v.X) + "[]"
	default:
		return "(expr)"
	}
}

// buildSpec emits a minimal but valid OpenAPI 3.1 document. Schemas are kept
// permissive (free-form object) so the first cut is complete; refinement of
// payload schemas is a follow-up that does not require touching routes.
func buildSpec(routes []route) ([]byte, error) {
	var b strings.Builder
	b.WriteString("openapi: 3.1.0\n")
	b.WriteString("info:\n")
	b.WriteString("  title: AIOps API\n")
	b.WriteString("  version: 0.19.67\n")
	b.WriteString("  description: Generated from cmd/server/handlers.go — do not edit by hand. Run `go run ./cmd/server/genapi`.\n")
	b.WriteString("paths:\n")

	// Group by path.
	byPath := map[string][]route{}
	var pathOrder []string
	for _, r := range routes {
		if _, ok := byPath[r.Path]; !ok {
			pathOrder = append(pathOrder, r.Path)
		}
		byPath[r.Path] = append(byPath[r.Path], r)
	}
	sort.Strings(pathOrder)

	for _, p := range pathOrder {
		rs := byPath[p]
		b.WriteString("  " + yamlQuotePath(p) + ":\n")
		for _, r := range rs {
			b.WriteString("    " + strings.ToLower(r.Method) + ":\n")
			b.WriteString("      operationId: " + sanitizeOpID(r.Handler) + "\n")
			b.WriteString("      summary: " + yamlQuoteString(r.Method+" "+p) + "\n")
			b.WriteString("      responses:\n")
			b.WriteString("        '200':\n")
			b.WriteString("          description: OK\n")
			b.WriteString("          content:\n")
			b.WriteString("            application/json:\n")
			b.WriteString("              schema:\n")
			b.WriteString("                type: object\n")
			b.WriteString("                additionalProperties: true\n")
			// Path parameters from {id} segments.
			params := pathParams(p)
			if len(params) > 0 {
				b.WriteString("      parameters:\n")
				for _, param := range params {
					b.WriteString("        - name: " + param + "\n")
					b.WriteString("          in: path\n")
					b.WriteString("          required: true\n")
					b.WriteString("          schema:\n")
					b.WriteString("            type: string\n")
				}
			}
		}
	}
	return []byte(b.String()), nil
}

// yamlQuotePath quotes a path that may contain { } characters so it stays a
// valid YAML key. Simple paths pass through unquoted.
func yamlQuotePath(p string) string {
	if strings.ContainsAny(p, "{}:") {
		return yamlQuoteString(p)
	}
	return p
}

func yamlQuoteString(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

func sanitizeOpID(h string) string {
	var b strings.Builder
	for _, r := range h {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "handler"
	}
	return b.String()
}

func pathParams(p string) []string {
	var out []string
	for _, seg := range strings.Split(p, "/") {
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			out = append(out, strings.Trim(seg, "{}"))
		}
	}
	return out
}

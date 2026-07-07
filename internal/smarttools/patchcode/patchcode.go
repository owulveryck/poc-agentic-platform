// Package patchcode implements the patch_code Smart Tool: targeted edition
// with in-tool validation, instead of letting the agent rewrite whole files
// blindly.
//
// PoC sandbox: the patched content is parsed with go/parser in an isolated
// buffer — nothing touches the real target. Production would apply the patch
// in a copy-on-write workspace and run the project linter/compiler.
package patchcode

import (
	"go/parser"
	"go/token"

	"github.com/owulveryck/poc-agentic-platform/internal/smarttools/translate"
)

// Tool is the patch_code Smart Tool.
type Tool struct{}

// ID implements smarttools.Tool.
func (Tool) ID() string { return "patch_code" }

// Run applies the patch in the sandbox and analyzes the result.
// payload: {"content": "<full patched Go file content>"}
func (Tool) Run(targets []string, payload map[string]any) map[string]any {
	content, _ := payload["content"].(string)
	if content == "" {
		return translate.Generic(1, "empty patch content")
	}

	// Sandbox execution: parse the patched content without touching disk.
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, targets[0], content, parser.AllErrors)

	// Semantic analysis of the outcome.
	if err != nil {
		base := translate.Generic(1, err.Error()) // compensatory translation
		return translate.SyntaxError(base, err.Error())
	}
	return map[string]any{"status": "OK", "targets": targets}
}

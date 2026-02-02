// SPDX-License-Identifier: AGPL-3.0-or-later

package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type goListPackage struct {
	ImportPath   string   `json:"ImportPath"`
	Imports      []string `json:"Imports"`
	TestImports  []string `json:"TestImports"`
	XTestImports []string `json:"XTestImports"`
}

func TestNoCoreImportsServer(t *testing.T) {
	moduleRoot, err := findModuleRoot()
	if err != nil {
		t.Fatalf("find module root: %v", err)
	}

	cmd := exec.Command(
		"go",
		"list",
		"-json",
		"./internal/engine/...",
		"./internal/coredb/...",
		"./internal/observability/...",
	)
	cmd.Dir = moduleRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list failed: %v\n%s", err, string(output))
	}

	decoder := json.NewDecoder(bytes.NewReader(output))
	var violations []string
	for decoder.More() {
		var pkg goListPackage
		if err := decoder.Decode(&pkg); err != nil {
			t.Fatalf("decode go list output: %v", err)
		}
		for _, imp := range collectImports(pkg) {
			if strings.HasPrefix(imp, "github.com/flowd-org/flowd/internal/server") {
				violations = append(violations, fmt.Sprintf("%s imports %s", pkg.ImportPath, imp))
			}
		}
	}

	if len(violations) > 0 {
		t.Fatalf("core packages must not import internal/server/*:\n%s", strings.Join(violations, "\n"))
	}
}

func collectImports(pkg goListPackage) []string {
	imports := append([]string{}, pkg.Imports...)
	imports = append(imports, pkg.TestImports...)
	imports = append(imports, pkg.XTestImports...)
	return imports
}

func findModuleRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("runtime.Caller failed")
	}

	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("go.mod not found")
		}
		dir = parent
	}
}

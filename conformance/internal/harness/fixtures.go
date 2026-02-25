package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// FixtureJobIDs returns the expected job IDs for the conformance fixtures.
func FixtureJobIDs() []string {
	return []string{
		"conformance/ulc-smoke-bash",
		"conformance/ulc-smoke-pwsh",
	}
}

// copyDir recursively copies a directory from src to dst using Go stdlib.
func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return fmt.Errorf("failed to create destination dir %s: %w", dst, err)
	}

	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return fmt.Errorf("failed to create dir %s: %w", filepath.Dir(target), err)
		}

		in, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open %s: %w", path, err)
		}
		defer in.Close()

		out, err := os.Create(target)
		if err != nil {
			return fmt.Errorf("failed to create %s: %w", target, err)
		}
		defer out.Close()

		if _, err := io.Copy(out, in); err != nil {
			return fmt.Errorf("failed to copy %s to %s: %w", path, target, err)
		}

		// Preserve mode bits, ensure .sh files are executable
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("failed to get info for %s: %w", path, err)
		}
		mode := info.Mode()
		if strings.HasSuffix(path, ".sh") {
			mode |= 0111 // ensure executable bits
		}
		if err := os.Chmod(target, mode); err != nil {
			return fmt.Errorf("failed to chmod %s: %w", target, err)
		}

		return nil
	})
}

// StageFixtures copies the fixture tree into the run root and returns the relative ref.
func StageFixtures(runRoot string) (stagedRef string, err error) {
	// Try candidate paths for the source fixture tree, including from internal/harness subdir.
	candidates := []string{
		filepath.Join("..", "..", "fixtures", "tree-v1"),
		filepath.Join("..", "fixtures", "tree-v1"),
		filepath.Join("fixtures", "tree-v1"),
		filepath.Join("conformance", "fixtures", "tree-v1"),
	}

	var src string
	for _, c := range candidates {
		if _, statErr := os.Stat(c); statErr == nil {
			src = c
			break
		}
	}
	if src == "" {
		return "", fmt.Errorf("fixture source not found in candidates: %v", candidates)
	}

	dst := filepath.Join(runRoot, "scripts", "fixtures", "tree-v1")

	// Remove existing destination if present
	if _, statErr := os.Stat(dst); statErr == nil {
		if rmErr := os.RemoveAll(dst); rmErr != nil {
			return "", fmt.Errorf("failed to remove existing fixtures: %w", rmErr)
		}
	}

	// Create destination directory
	if mkErr := os.MkdirAll(filepath.Dir(dst), 0755); mkErr != nil {
		return "", fmt.Errorf("failed to create fixtures directory: %w", mkErr)
	}

	// Copy the fixture tree using Go stdlib
	if copyErr := copyDir(src, dst); copyErr != nil {
		return "", fmt.Errorf("failed to copy fixtures: %w", copyErr)
	}

	// Ensure .sh files are executable (in case source mode bits differ)
	if err := filepath.WalkDir(dst, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".sh") {
			if chmodErr := os.Chmod(path, 0755); chmodErr != nil {
				return fmt.Errorf("failed to chmod %s: %w", path, chmodErr)
			}
		}
		return nil
	}); err != nil {
		return "", fmt.Errorf("failed to walk and chmod .sh files: %w", err)
	}

	return "fixtures/tree-v1", nil
}

// RegisterLocalSource registers a local source with the flowd harness.
func RegisterLocalSource(ctx context.Context, c *Client, name string, ref string) error {
	payload := map[string]string{
		"type": "local",
		"name": name,
		"ref":  ref,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := c.NewRequest(ctx, http.MethodPost, "/sources", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	// req.Header.Set("Content-Type", "application/json") // already set by NewRequest via Accept header, but we need Content-Type for POST
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("failed to POST /sources: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Read a limited body for diagnostics (redacted)
		buf := make([]byte, maxBodyRead)
		n, _ := resp.Body.Read(buf)
		diag := RedactSecrets(string(buf[:n]), c.Token)
		return fmt.Errorf("POST /sources failed with status %d: %s", resp.StatusCode, diag)
	}

	return nil
}

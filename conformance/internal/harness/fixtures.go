package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
)

// FixtureJobIDs returns the expected job IDs for the conformance fixtures.
func FixtureJobIDs() []string {
	return []string{
		"conformance/ulc-smoke-bash",
		"conformance/ulc-smoke-pwsh",
	}
}

// StageFixtures copies the fixture tree into the run root and returns the relative ref.
func StageFixtures(runRoot string) (stagedRef string, err error) {
	src := filepath.Join("conformance", "fixtures", "tree-v1")
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

	// Copy the fixture tree
	cmd := exec.Command("cp", "-r", src, dst)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to copy fixtures: %w, output: %s", err, out)
	}

	// Ensure .sh files are executable
	shFiles, err := filepath.Glob(filepath.Join(dst, "**", "*.sh"))
	if err != nil {
		return "", fmt.Errorf("failed to glob .sh files: %w", err)
	}
	for _, f := range shFiles {
		if chmodErr := os.Chmod(f, 0755); chmodErr != nil {
			return "", fmt.Errorf("failed to chmod %s: %w", f, chmodErr)
		}
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/sources", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(c.Token, "")

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

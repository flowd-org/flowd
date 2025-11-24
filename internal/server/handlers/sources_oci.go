// SPDX-License-Identifier: AGPL-3.0-or-later
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/flowd-org/flowd/internal/executor/container"
	"github.com/flowd-org/flowd/internal/paths"
)

func resolveRuntimeForOCI(ctx context.Context, cfg SourcesConfig) (container.Runtime, string, error) {
	runtimeVal := cfg.Runtime
	var err error
	if runtimeVal == "" {
		if cfg.RuntimeDetector != nil {
			runtimeVal, err = cfg.RuntimeDetector()
		} else {
			runtimeVal, err = container.DetectRuntime(nil)
		}
	}
	if err != nil {
		return "", "", err
	}
	if runtimeVal == "" {
		return "", "", errors.New("no container runtime configured")
	}
	return runtimeVal, string(runtimeVal), nil
}

func pullOCIImage(ctx context.Context, runtime container.Runtime, image string) error {
	if runtime == "" {
		return errors.New("container runtime required for pull")
	}
	output, err := ociRuntimeCommand(ctx, runtime, "pull", image)
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			detail = err.Error()
		}
		return fmt.Errorf("%w: %s", errOCIPullFailure, detail)
	}
	return nil
}

func extractAddonManifest(ctx context.Context, runtime container.Runtime, image, profile, pullPolicy string) ([]byte, error) {
	if runtime == "" {
		return nil, errors.New("container runtime required for manifest extraction")
	}
	args := []string{"run", "--rm"}
	switch pullPolicy {
	case "on-run":
		args = append(args, "--pull=never")
	default:
		args = append(args, "--pull=always")
	}

	// Hardened defaults; disabled profile may opt into writable rootfs/network.
	args = append(args, "--cap-drop=ALL", "--security-opt=no-new-privileges")
	if strings.EqualFold(profile, "disabled") {
		args = append(args, "--network", "bridge")
	} else {
		args = append(args, "--network", "none", "--read-only")
	}

	args = append(args, "--entrypoint", "cat")
	args = append(args, image, addonManifestMountPath)

	output, err := ociRuntimeCommand(ctx, runtime, args...)
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			detail = err.Error()
		}
		if strings.Contains(strings.ToLower(detail), "no such file") || strings.Contains(strings.ToLower(detail), "not found") {
			return nil, fmt.Errorf("%w: %s", errManifestMissing, detail)
		}
		return nil, fmt.Errorf("%w: %s", errOCICommandFailure, detail)
	}
	return output, nil
}

func inspectImageMetadata(ctx context.Context, runtime container.Runtime, image string) (ociImageMetadata, error) {
	if runtime == "" {
		return ociImageMetadata{}, errors.New("container runtime required for inspect")
	}
	output, err := ociRuntimeCommand(ctx, runtime, "image", "inspect", image)
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			detail = err.Error()
		}
		return ociImageMetadata{}, fmt.Errorf("%w: %s", errOCICommandFailure, detail)
	}

	var payload []struct {
		Digest      string   `json:"Digest"`
		RepoDigests []string `json:"RepoDigests"`
		Created     string   `json:"Created"`
		ID          string   `json:"Id"`
		IDAlt       string   `json:"ID"`
		Size        int64    `json:"Size"`
		Config      struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
	}
	if unmarshalErr := json.Unmarshal(output, &payload); unmarshalErr != nil {
		return ociImageMetadata{}, fmt.Errorf("parse inspect payload: %w", unmarshalErr)
	}
	if len(payload) == 0 {
		return ociImageMetadata{}, errors.New("image inspect returned no results")
	}
	entry := payload[0]
	meta := ociImageMetadata{
		Digest:    strings.TrimSpace(entry.Digest),
		Created:   strings.TrimSpace(entry.Created),
		ImageID:   strings.TrimSpace(entry.ID),
		SizeBytes: entry.Size,
		Labels:    entry.Config.Labels,
	}
	if meta.ImageID == "" {
		meta.ImageID = strings.TrimSpace(entry.IDAlt)
	}
	if meta.Digest == "" {
		for _, repo := range entry.RepoDigests {
			repo = strings.TrimSpace(repo)
			if repo == "" {
				continue
			}
			if at := strings.Index(repo, "@"); at >= 0 && at+1 < len(repo) {
				meta.Digest = strings.TrimSpace(repo[at+1:])
				break
			}
		}
	}
	if meta.Digest == "" {
		return ociImageMetadata{}, errors.New("image digest not available")
	}
	if meta.Labels == nil {
		meta.Labels = map[string]string{}
	}
	return meta, nil
}

func deriveOCICacheRoot(checkoutDir string) string {
	if checkoutDir == "" {
		return paths.OCICacheDir()
	}
	base := filepath.Dir(checkoutDir)
	if base == "" || base == "." || base == string(filepath.Separator) {
		return filepath.Join(checkoutDir, ociCacheDirName)
	}
	return filepath.Join(base, ociCacheDirName)
}

func writeAddonManifest(cacheRoot, name string, manifest []byte) (string, error) {
	if cacheRoot == "" {
		cacheRoot = paths.OCICacheDir()
	}
	if err := os.MkdirAll(cacheRoot, 0o755); err != nil {
		return "", fmt.Errorf("create cache root: %w", err)
	}
	targetDir := filepath.Join(cacheRoot, name)
	if !isSubPath(targetDir, cacheRoot) {
		return "", fmt.Errorf("invalid source name %q", name)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", fmt.Errorf("create source cache dir: %w", err)
	}
	manifestPath := filepath.Join(targetDir, addonManifestFileName)
	if err := os.WriteFile(manifestPath, manifest, 0o600); err != nil {
		return "", fmt.Errorf("write manifest: %w", err)
	}
	return manifestPath, nil
}

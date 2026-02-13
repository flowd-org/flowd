// SPDX-License-Identifier: AGPL-3.0-or-later

package artifacts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/flowd-org/flowd/internal/coredb"
	"github.com/flowd-org/flowd/internal/paths"
)

const (
	defaultMaxArtifactBytes int64 = 256 << 20 // 256 MiB
)

var (
	// ErrImmutableWrite indicates an artifact_id that already has bytes on disk.
	ErrImmutableWrite = errors.New("artifacts: immutable write violation")
	// ErrArtifactTooLarge indicates the payload exceeds the configured max size.
	ErrArtifactTooLarge = errors.New("artifacts: payload exceeds maximum size")
)

// Store writes immutable artifact bytes under the instance data directory.
type Store struct {
	rootDir          string
	maxArtifactBytes int64
}

// Options controls artifact byte-store behavior.
type Options struct {
	RootDir          string
	MaxArtifactBytes int64
}

// NewStore returns a filesystem-backed artifact byte store.
func NewStore(opts Options) *Store {
	root := strings.TrimSpace(opts.RootDir)
	if root == "" {
		root = paths.DataPath("artifacts")
	}
	maxBytes := opts.MaxArtifactBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxArtifactBytes
	}
	return &Store{rootDir: root, maxArtifactBytes: maxBytes}
}

// Write stores artifact bytes once, enforcing immutability and size limits.
func (s *Store) Write(ctx context.Context, artifactID string, src io.Reader) (int64, error) {
	if s == nil {
		return 0, errors.New("artifacts: store unavailable")
	}
	normalizedID, err := coredb.NormalizeArtifactIDForStorage(artifactID)
	if err != nil {
		return 0, err
	}
	if src == nil {
		return 0, errors.New("artifacts: source reader required")
	}

	targetPath := s.pathForID(normalizedID)
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o700); err != nil {
		return 0, fmt.Errorf("artifacts: ensure parent dir: %w", err)
	}

	f, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return 0, ErrImmutableWrite
		}
		return 0, fmt.Errorf("artifacts: open target: %w", err)
	}
	defer f.Close()

	limitReader := io.LimitReader(src, s.maxArtifactBytes+1)
	written, err := copyWithContext(ctx, f, limitReader)
	if err != nil {
		_ = os.Remove(targetPath)
		return 0, err
	}
	if written > s.maxArtifactBytes {
		_ = os.Remove(targetPath)
		return 0, ErrArtifactTooLarge
	}
	return written, nil
}

// Open opens immutable artifact bytes for reading.
func (s *Store) Open(artifactID string) (*os.File, error) {
	if s == nil {
		return nil, errors.New("artifacts: store unavailable")
	}
	normalizedID, err := coredb.NormalizeArtifactIDForStorage(artifactID)
	if err != nil {
		return nil, err
	}
	return os.Open(s.pathForID(normalizedID))
}

func (s *Store) pathForID(artifactID string) string {
	prefix := artifactID[:2]
	return filepath.Join(s.rootDir, prefix, artifactID)
}

func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	if ctx == nil {
		return io.Copy(dst, src)
	}
	buf := make([]byte, 32*1024)
	var total int64
	for {
		select {
		case <-ctx.Done():
			return total, ctx.Err()
		default:
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			written, writeErr := dst.Write(buf[:n])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
			if written != n {
				return total, io.ErrShortWrite
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return total, nil
			}
			return total, readErr
		}
	}
}

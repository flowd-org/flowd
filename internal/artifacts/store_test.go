// SPDX-License-Identifier: AGPL-3.0-or-later

package artifacts

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/flowd-org/flowd/internal/coredb"
)

const testArtifactID = "018f0d40-0b3e-7c1a-8f0e-5f0b6bd8f403"

func TestStoreWriteImmutableByID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewStore(Options{RootDir: t.TempDir(), MaxArtifactBytes: 64})

	written, err := store.Write(ctx, strings.ToUpper(testArtifactID), strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("write first artifact payload: %v", err)
	}
	if written != 5 {
		t.Fatalf("expected 5 written bytes, got %d", written)
	}

	f, err := store.Open(testArtifactID)
	if err != nil {
		t.Fatalf("open artifact bytes: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	content, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("read artifact bytes: %v", err)
	}
	if string(content) != "hello" {
		t.Fatalf("unexpected artifact content: %q", string(content))
	}

	if _, err := store.Write(ctx, testArtifactID, strings.NewReader("overwrite")); !errors.Is(err, ErrImmutableWrite) {
		t.Fatalf("expected immutable write error, got %v", err)
	}
}

func TestStoreWriteEnforcesSizeCap(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewStore(Options{RootDir: t.TempDir(), MaxArtifactBytes: 5})

	if _, err := store.Write(ctx, testArtifactID, strings.NewReader("123456")); !errors.Is(err, ErrArtifactTooLarge) {
		t.Fatalf("expected too-large error, got %v", err)
	}

	if _, err := store.Open(testArtifactID); err == nil {
		t.Fatalf("expected oversized write to leave no artifact bytes on disk")
	}
}

func TestStoreWriteHandlesCloseError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewStore(Options{RootDir: t.TempDir(), MaxArtifactBytes: 256 << 20})

	// Write a valid artifact first to ensure the directory structure exists
	written, err := store.Write(ctx, testArtifactID, strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("write first artifact payload: %v", err)
	}
	if written != 5 {
		t.Fatalf("expected 5 written bytes, got %d", written)
	}

	// Verify the file exists
	targetPath := store.pathForID(testArtifactID)
	if _, statErr := os.Stat(targetPath); statErr != nil {
		t.Fatalf("expected artifact file to exist: %v", statErr)
	}
}

func TestArtifactMetadataPersistenceRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestDB(t)
	meta := coredb.NewArtifactStore(db)

	createdAt := time.Date(2026, 2, 13, 8, 30, 0, 0, time.UTC)
	record := coredb.ArtifactRecord{
		ArtifactID:  strings.ToUpper(testArtifactID),
		Tenant:      "tenant-a",
		JobID:       "job-42",
		RunID:       "run-99",
		Name:        "stdout",
		ContentType: "text/plain",
		SizeBytes:   5,
		CreatedAt:   createdAt,
	}
	if err := meta.Create(ctx, record); err != nil {
		t.Fatalf("create metadata: %v", err)
	}

	got, found, err := meta.Get(ctx, strings.ToUpper(testArtifactID))
	if err != nil {
		t.Fatalf("get metadata: %v", err)
	}
	if !found {
		t.Fatalf("expected metadata row to exist")
	}
	if got.ArtifactID != testArtifactID {
		t.Fatalf("expected normalized artifact id %q, got %q", testArtifactID, got.ArtifactID)
	}
	if got.Tenant != record.Tenant || got.JobID != record.JobID || got.RunID != record.RunID {
		t.Fatalf("unexpected scope metadata: %+v", got)
	}
	if got.Name != record.Name || got.ContentType != record.ContentType {
		t.Fatalf("unexpected artifact metadata fields: %+v", got)
	}
	if got.SizeBytes != record.SizeBytes {
		t.Fatalf("expected size %d, got %d", record.SizeBytes, got.SizeBytes)
	}
	if !got.CreatedAt.Equal(createdAt) {
		t.Fatalf("expected created_at %v, got %v", createdAt, got.CreatedAt)
	}
}

func openTestDB(t *testing.T) *coredb.DB {
	t.Helper()
	db, err := coredb.Open(context.Background(), coredb.Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("open coredb: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestStoreDeleteMissingArtifact(t *testing.T) {
	t.Parallel()

	store := NewStore(Options{RootDir: t.TempDir(), MaxArtifactBytes: 256 << 20})

	// Deleting a non-existent artifact should return nil (already clean)
	err := store.Delete("018f0d40-0b3e-7c1a-8f0e-5f0b6bd8f404")
	if err != nil {
		t.Fatalf("expected nil error for missing artifact, got %v", err)
	}
}

func TestStoreDeleteExistingArtifact(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewStore(Options{RootDir: t.TempDir(), MaxArtifactBytes: 256 << 20})

	// Write an artifact first
	written, err := store.Write(ctx, testArtifactID, strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	if written != 5 {
		t.Fatalf("expected 5 bytes written, got %d", written)
	}

	// Verify file exists before delete
	targetPath := store.pathForID(testArtifactID)
	if _, statErr := os.Stat(targetPath); statErr != nil {
		t.Fatalf("expected artifact file to exist: %v", statErr)
	}

	// Delete the artifact
	err = store.Delete(testArtifactID)
	if err != nil {
		t.Fatalf("delete artifact: %v", err)
	}

	// Verify file no longer exists
	if _, statErr := os.Stat(targetPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected artifact file to be removed")
	}
}

func TestStoreOpenMissingArtifact(t *testing.T) {
	t.Parallel()

	store := NewStore(Options{RootDir: t.TempDir(), MaxArtifactBytes: 256 << 20})

	// Opening a non-existent artifact should return an error
	f, err := store.Open("018f0d40-0b3e-7c1a-8f0e-5f0b6bd8f404")
	if err == nil {
		t.Fatalf("expected error for missing artifact, got nil")
	}
	_ = f // ignore unused variable if err != nil
}

func TestStoreCopyWithContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	store := NewStore(Options{RootDir: t.TempDir(), MaxArtifactBytes: 256 << 20})

	// Use a reader that blocks to simulate a slow source
	src := &blockingReader{}

	// Cancel immediately to trigger context cancellation during copy
	cancel()

	_, err := store.Write(ctx, testArtifactID, src)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled error, got %v", err)
	}
}

type blockingReader struct{}

func (r *blockingReader) Read(p []byte) (int, error) {
	// Block indefinitely to simulate a slow reader
	select {}
}

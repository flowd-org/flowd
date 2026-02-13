// SPDX-License-Identifier: AGPL-3.0-or-later

package artifacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/flowd-org/flowd/internal/coredb"
)

const testArtifactIDTwo = "018f0d40-0b3e-7c1a-8f0e-5f0b6bd8f404"

func TestDeriveMappingKeyTrimsScopeAndNormalizesArtifactKey(t *testing.T) {
	t.Parallel()

	got, err := DeriveMappingKey(" TENANT-A ", " Job-42 ", "Backups/Latest")
	if err != nil {
		t.Fatalf("derive mapping key: %v", err)
	}

	raw := "TENANT-A\nJob-42\nbackups/latest"
	sum := sha256.Sum256([]byte(raw))
	want := "ak/" + hex.EncodeToString(sum[:])
	if got != want {
		t.Fatalf("unexpected derived key: got %q want %q", got, want)
	}
}

func TestKeyMappingStoreRejectsInvalidArtifactKey(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestDB(t)
	store := NewKeyMappingStore(db)

	writeArtifactWithMetadata(t, db, "tenant-a", "job-42", testArtifactID, "stdout", "payload-1")

	err := store.Set(ctx, "tenant-a", "job-42", "bad key with spaces", testArtifactID)
	if err == nil {
		t.Fatalf("expected invalid key error")
	}
	if !errors.Is(err, ErrMappingInvalidScope) {
		t.Fatalf("expected ErrMappingInvalidScope, got %v", err)
	}
}

func TestKeyMappingStoreScopesByTenantAndJob(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestDB(t)
	store := NewKeyMappingStore(db)

	writeArtifactWithMetadata(t, db, "tenant-a", "job-42", testArtifactID, "stdout", "payload-1")
	if err := store.Set(ctx, "tenant-a", "job-42", "Backups/Latest", testArtifactID); err != nil {
		t.Fatalf("set mapping: %v", err)
	}

	resolved, found, err := store.Resolve(ctx, "tenant-a", "job-42", "backups/latest")
	if err != nil {
		t.Fatalf("resolve scoped mapping: %v", err)
	}
	if !found {
		t.Fatalf("expected scoped mapping to resolve")
	}
	if resolved != testArtifactID {
		t.Fatalf("unexpected resolved artifact id: got %q want %q", resolved, testArtifactID)
	}

	_, found, err = store.Resolve(ctx, "TENANT-A", "job-42", "backups/latest")
	if err != nil {
		t.Fatalf("resolve with tenant case mismatch: %v", err)
	}
	if found {
		t.Fatalf("expected tenant case mismatch lookup to be isolated")
	}

	_, found, err = store.Resolve(ctx, "tenant-b", "job-42", "backups/latest")
	if err != nil {
		t.Fatalf("resolve with different tenant: %v", err)
	}
	if found {
		t.Fatalf("expected tenant mismatch lookup to be isolated")
	}

	_, found, err = store.Resolve(ctx, "tenant-a", "job-43", "backups/latest")
	if err != nil {
		t.Fatalf("resolve with different job: %v", err)
	}
	if found {
		t.Fatalf("expected job mismatch lookup to be isolated")
	}
}

func TestKeyMappingStoreSetRejectsTenantCaseMismatch(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestDB(t)
	store := NewKeyMappingStore(db)

	writeArtifactWithMetadata(t, db, "tenant-a", "job-42", testArtifactID, "stdout", "payload-1")
	err := store.Set(ctx, "TENANT-A", "job-42", "backups/latest", testArtifactID)
	if !errors.Is(err, ErrMappingScopeMismatch) {
		t.Fatalf("expected ErrMappingScopeMismatch, got %v", err)
	}
}

func TestKeyMappingStoreRepointDoesNotMutateArtifactBytes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestDB(t)
	store := NewKeyMappingStore(db)
	bytesStore := NewStore(Options{RootDir: t.TempDir(), MaxArtifactBytes: 1024})

	writeArtifactWithMetadata(t, db, "tenant-a", "job-42", testArtifactID, "stdout", "payload-1")
	if _, err := bytesStore.Write(ctx, testArtifactID, strings.NewReader("payload-1")); err != nil {
		t.Fatalf("write artifact one bytes: %v", err)
	}

	writeArtifactWithMetadata(t, db, "tenant-a", "job-42", testArtifactIDTwo, "stderr", "payload-2")
	if _, err := bytesStore.Write(ctx, testArtifactIDTwo, strings.NewReader("payload-2")); err != nil {
		t.Fatalf("write artifact two bytes: %v", err)
	}

	if err := store.Set(ctx, "tenant-a", "job-42", "backups/latest", testArtifactID); err != nil {
		t.Fatalf("set initial mapping: %v", err)
	}
	if err := store.Set(ctx, "tenant-a", "job-42", "backups/latest", testArtifactIDTwo); err != nil {
		t.Fatalf("repoint mapping: %v", err)
	}

	resolved, found, err := store.Resolve(ctx, "tenant-a", "job-42", "backups/latest")
	if err != nil {
		t.Fatalf("resolve repointed mapping: %v", err)
	}
	if !found {
		t.Fatalf("expected repointed mapping to resolve")
	}
	if resolved != testArtifactIDTwo {
		t.Fatalf("expected mapping to resolve to new artifact id %q, got %q", testArtifactIDTwo, resolved)
	}

	oneBytes := readArtifactBytes(t, bytesStore, testArtifactID)
	if string(oneBytes) != "payload-1" {
		t.Fatalf("expected original artifact bytes to stay unchanged, got %q", string(oneBytes))
	}

	twoBytes := readArtifactBytes(t, bytesStore, testArtifactIDTwo)
	if string(twoBytes) != "payload-2" {
		t.Fatalf("expected repointed artifact bytes to stay unchanged, got %q", string(twoBytes))
	}
}

func writeArtifactWithMetadata(t *testing.T, db *coredb.DB, tenant, jobID, artifactID, name, payload string) {
	t.Helper()

	meta := coredb.NewArtifactStore(db)
	record := coredb.ArtifactRecord{
		ArtifactID:  artifactID,
		Tenant:      tenant,
		JobID:       jobID,
		RunID:       "run-1",
		Name:        name,
		ContentType: "text/plain",
		SizeBytes:   int64(len(payload)),
		CreatedAt:   time.Date(2026, 2, 13, 9, 0, 0, 0, time.UTC),
	}
	if err := meta.Create(context.Background(), record); err != nil {
		t.Fatalf("create artifact metadata: %v", err)
	}
}

func readArtifactBytes(t *testing.T, store *Store, artifactID string) []byte {
	t.Helper()

	f, err := store.Open(artifactID)
	if err != nil {
		t.Fatalf("open artifact %q: %v", artifactID, err)
	}
	t.Cleanup(func() { _ = f.Close() })
	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("read artifact %q: %v", artifactID, err)
	}
	return b
}

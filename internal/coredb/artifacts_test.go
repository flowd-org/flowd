// SPDX-License-Identifier: AGPL-3.0-or-later

package coredb

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestNormalizeArtifactID(t *testing.T) {
	t.Run("valid uuidv7 rfc4122", func(t *testing.T) {
		u, err := uuid.NewV7()
		assert.NoError(t, err)
		idStr := u.String()

		result, err := normalizeArtifactID(idStr)
		assert.NoError(t, err)
		assert.Equal(t, idStr, result)
	})

	t.Run("uuidv7 with rfc4122 variant from string", func(t *testing.T) {
		s := "018f22b0-1234-7abc-8def-0123456789ab"
		result, err := normalizeArtifactID(s)
		assert.NoError(t, err)
		assert.Equal(t, s, result)
	})

	t.Run("uuidv4 rfc4122 variant rejected", func(t *testing.T) {
		s := "550e8400-e29b-41d4-a716-446655440000" // v4 RFC4122
		result, err := normalizeArtifactID(s)
		assert.ErrorIs(t, err, ErrArtifactInvalidID)
		assert.Empty(t, result)
	})

	t.Run("uuidv5 variant 2 rejected", func(t *testing.T) {
		s := "12345678-1234-5678-1234-56789abcdef0" // v5 variant 2 (DCE)
		result, err := normalizeArtifactID(s)
		assert.ErrorIs(t, err, ErrArtifactInvalidID)
		assert.Empty(t, result)
	})

	t.Run("uuidv6 rfc4122 variant rejected", func(t *testing.T) {
		s := "00000000-0000-6000-8000-000000000000" // v6 RFC4122
		result, err := normalizeArtifactID(s)
		assert.ErrorIs(t, err, ErrArtifactInvalidID)
		assert.Empty(t, result)
	})

	t.Run("invalid uuid format rejected", func(t *testing.T) {
		s := "not-a-uuid"
		result, err := normalizeArtifactID(s)
		assert.ErrorIs(t, err, ErrArtifactInvalidID)
		assert.Empty(t, result)
	})

	t.Run("trims whitespace", func(t *testing.T) {
		s := "  018f22b0-1234-7abc-8def-0123456789ab  "
		result, err := normalizeArtifactID(s)
		assert.NoError(t, err)
		assert.Equal(t, "018f22b0-1234-7abc-8def-0123456789ab", result)
	})
}

func TestNormalizeArtifactRecordWithVariantCheck(t *testing.T) {
	t.Run("valid record with uuidv7 rfc4122", func(t *testing.T) {
		u, err := uuid.NewV7()
		assert.NoError(t, err)

		record := ArtifactRecord{
			ArtifactID:  u.String(),
			Tenant:      "acme",
			JobID:       "demo",
			RunID:       "run-1",
			Name:        "stdout",
			ContentType: "text/plain",
			SizeBytes:   1024,
		}

		result, err := normalizeArtifactRecord(record, nil)
		assert.NoError(t, err)
		assert.Equal(t, u.String(), result.ArtifactID)
	})

	t.Run("invalid record with uuidv4 rejected", func(t *testing.T) {
		record := ArtifactRecord{
			ArtifactID:  "550e8400-e29b-41d4-a716-446655440000",
			Tenant:      "acme",
			JobID:       "demo",
			RunID:       "run-1",
			Name:        "stdout",
			ContentType: "text/plain",
			SizeBytes:   1024,
		}

		result, err := normalizeArtifactRecord(record, nil)
		assert.ErrorIs(t, err, ErrArtifactInvalidID)
		assert.Equal(t, ArtifactRecord{}, result)
	})
}

func TestNewArtifactStore(t *testing.T) {
	t.Run("nil db returns nil", func(t *testing.T) {
		store := NewArtifactStore(nil)
		assert.Nil(t, store)
	})

	t.Run("valid db returns store with now function", func(t *testing.T) {
		db := openTestDB(t)
		defer func() { _ = db.Close() }()

		store := NewArtifactStore(db)
		assert.NotNil(t, store)
		assert.NotNil(t, store.now)
	})
}

func TestArtifactStoreCreate(t *testing.T) {
	t.Run("uninitialized store returns error", func(t *testing.T) {
		store := &ArtifactStore{}
		err := store.Create(context.Background(), ArtifactRecord{})
		assert.ErrorIs(t, err, ErrArtifactStoreUnavailable)
	})

	t.Run("nil artifact store returns error", func(t *testing.T) {
		var store *ArtifactStore
		err := store.Create(context.Background(), ArtifactRecord{})
		assert.ErrorIs(t, err, ErrArtifactStoreUnavailable)
	})

	t.Run("valid record creates entry", func(t *testing.T) {
		db := openTestDB(t)
		defer func() { _ = db.Close() }()

		store := NewArtifactStore(db)
		u, err := uuid.NewV7()
		assert.NoError(t, err)

		record := ArtifactRecord{
			ArtifactID:  u.String(),
			Tenant:      "acme",
			JobID:       "demo",
			RunID:       "run-1",
			Name:        "stdout",
			ContentType: "text/plain",
			SizeBytes:   1024,
		}

		err = store.Create(context.Background(), record)
		assert.NoError(t, err)

		// Verify the record was created
		stored, found, err := store.Get(context.Background(), u.String())
		assert.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, "acme", stored.Tenant)
	})
}

func TestArtifactStoreGet(t *testing.T) {
	t.Run("uninitialized store returns error", func(t *testing.T) {
		store := &ArtifactStore{}
		u, err := uuid.NewV7()
		assert.NoError(t, err)
		record, found, err := store.Get(context.Background(), u.String())
		assert.ErrorIs(t, err, ErrArtifactStoreUnavailable)
		assert.False(t, found)
		assert.Equal(t, ArtifactRecord{}, record)
	})

	t.Run("nil artifact store returns error", func(t *testing.T) {
		var store *ArtifactStore
		u, err := uuid.NewV7()
		assert.NoError(t, err)
		record, found, err := store.Get(context.Background(), u.String())
		assert.ErrorIs(t, err, ErrArtifactStoreUnavailable)
		assert.False(t, found)
		assert.Equal(t, ArtifactRecord{}, record)
	})

	t.Run("non-existent artifact returns not found", func(t *testing.T) {
		db := openTestDB(t)
		defer func() { _ = db.Close() }()

		store := NewArtifactStore(db)
		u, err := uuid.NewV7()
		assert.NoError(t, err)

		record, found, err := store.Get(context.Background(), u.String())
		assert.NoError(t, err)
		assert.False(t, found)
		assert.Equal(t, ArtifactRecord{}, record)
	})

	t.Run("valid artifact returns record", func(t *testing.T) {
		db := openTestDB(t)
		defer func() { _ = db.Close() }()

		store := NewArtifactStore(db)
		u, err := uuid.NewV7()
		assert.NoError(t, err)

		record := ArtifactRecord{
			ArtifactID:  u.String(),
			Tenant:      "acme",
			JobID:       "demo",
			RunID:       "run-1",
			Name:        "stdout",
			ContentType: "text/plain",
			SizeBytes:   1024,
		}

		err = store.Create(context.Background(), record)
		assert.NoError(t, err)

		stored, found, err := store.Get(context.Background(), u.String())
		assert.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, "acme", stored.Tenant)
		assert.Equal(t, int64(1024), stored.SizeBytes)
	})

	t.Run("invalid artifact ID returns error", func(t *testing.T) {
		db := openTestDB(t)
		defer func() { _ = db.Close() }()

		store := NewArtifactStore(db)
		record, found, err := store.Get(context.Background(), "not-a-uuid")
		assert.ErrorIs(t, err, ErrArtifactInvalidID)
		assert.False(t, found)
		assert.Equal(t, ArtifactRecord{}, record)
	})
}

func TestNormalizeArtifactIDForStorage(t *testing.T) {
	t.Run("valid uuidv7 returns normalized", func(t *testing.T) {
		u, err := uuid.NewV7()
		assert.NoError(t, err)

		result, err := NormalizeArtifactIDForStorage(u.String())
		assert.NoError(t, err)
		assert.Equal(t, u.String(), result)
	})

	t.Run("invalid uuid returns error", func(t *testing.T) {
		result, err := NormalizeArtifactIDForStorage("not-a-uuid")
		assert.ErrorIs(t, err, ErrArtifactInvalidID)
		assert.Empty(t, result)
	})

	t.Run("trims whitespace", func(t *testing.T) {
		s := "  018f22b0-1234-7abc-8def-0123456789ab  "
		result, err := NormalizeArtifactIDForStorage(s)
		assert.NoError(t, err)
		assert.Equal(t, "018f22b0-1234-7abc-8def-0123456789ab", result)
	})
}

func TestNormalizeArtifactRecord(t *testing.T) {
	t.Run("empty tenant rejected", func(t *testing.T) {
		u, err := uuid.NewV7()
		assert.NoError(t, err)
		record := ArtifactRecord{
			ArtifactID:  u.String(),
			Tenant:      "",
			JobID:       "demo",
			RunID:       "run-1",
			Name:        "stdout",
			ContentType: "text/plain",
			SizeBytes:   1024,
		}
		result, err := normalizeArtifactRecord(record, nil)
		assert.ErrorIs(t, err, ErrArtifactInvalidMetadata)
		assert.Equal(t, ArtifactRecord{}, result)
	})

	t.Run("empty job_id rejected", func(t *testing.T) {
		u, err := uuid.NewV7()
		assert.NoError(t, err)
		record := ArtifactRecord{
			ArtifactID:  u.String(),
			Tenant:      "acme",
			JobID:       "",
			RunID:       "run-1",
			Name:        "stdout",
			ContentType: "text/plain",
			SizeBytes:   1024,
		}
		result, err := normalizeArtifactRecord(record, nil)
		assert.ErrorIs(t, err, ErrArtifactInvalidMetadata)
		assert.Equal(t, ArtifactRecord{}, result)
	})

	t.Run("empty run_id rejected", func(t *testing.T) {
		u, err := uuid.NewV7()
		assert.NoError(t, err)
		record := ArtifactRecord{
			ArtifactID:  u.String(),
			Tenant:      "acme",
			JobID:       "demo",
			RunID:       "",
			Name:        "stdout",
			ContentType: "text/plain",
			SizeBytes:   1024,
		}
		result, err := normalizeArtifactRecord(record, nil)
		assert.ErrorIs(t, err, ErrArtifactInvalidMetadata)
		assert.Equal(t, ArtifactRecord{}, result)
	})

	t.Run("empty name rejected", func(t *testing.T) {
		u, err := uuid.NewV7()
		assert.NoError(t, err)
		record := ArtifactRecord{
			ArtifactID:  u.String(),
			Tenant:      "acme",
			JobID:       "demo",
			RunID:       "run-1",
			Name:        "",
			ContentType: "text/plain",
			SizeBytes:   1024,
		}
		result, err := normalizeArtifactRecord(record, nil)
		assert.ErrorIs(t, err, ErrArtifactInvalidMetadata)
		assert.Equal(t, ArtifactRecord{}, result)
	})

	t.Run("negative size_bytes rejected", func(t *testing.T) {
		u, err := uuid.NewV7()
		assert.NoError(t, err)
		record := ArtifactRecord{
			ArtifactID:  u.String(),
			Tenant:      "acme",
			JobID:       "demo",
			RunID:       "run-1",
			Name:        "stdout",
			ContentType: "text/plain",
			SizeBytes:   -1,
		}
		result, err := normalizeArtifactRecord(record, nil)
		assert.ErrorIs(t, err, ErrArtifactInvalidMetadata)
		assert.Equal(t, ArtifactRecord{}, result)
	})

	t.Run("zero size_bytes accepted", func(t *testing.T) {
		u, err := uuid.NewV7()
		assert.NoError(t, err)
		record := ArtifactRecord{
			ArtifactID:  u.String(),
			Tenant:      "acme",
			JobID:       "demo",
			RunID:       "run-1",
			Name:        "stdout",
			ContentType: "text/plain",
			SizeBytes:   0,
		}
		result, err := normalizeArtifactRecord(record, nil)
		assert.NoError(t, err)
		assert.Equal(t, int64(0), result.SizeBytes)
	})

	t.Run("trims whitespace from fields", func(t *testing.T) {
		u1, err := uuid.NewV7()
		assert.NoError(t, err)

		record := ArtifactRecord{
			ArtifactID:  "  " + u1.String() + "  ",
			Tenant:      "  acme  ",
			JobID:       "  demo  ",
			RunID:       "  run-1  ",
			Name:        "  stdout  ",
			ContentType: "  text/plain  ",
			SizeBytes:   1024,
		}
		result, err := normalizeArtifactRecord(record, nil)
		assert.NoError(t, err)
		assert.Equal(t, "acme", result.Tenant)
		assert.Equal(t, "demo", result.JobID)
		assert.Equal(t, "run-1", result.RunID)
		assert.Equal(t, "stdout", result.Name)
		assert.Equal(t, "text/plain", result.ContentType)
	})

	t.Run("uses now function for zero CreatedAt", func(t *testing.T) {
		now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
		u, err := uuid.NewV7()
		assert.NoError(t, err)
		record := ArtifactRecord{
			ArtifactID:  u.String(),
			Tenant:      "acme",
			JobID:       "demo",
			RunID:       "run-1",
			Name:        "stdout",
			ContentType: "text/plain",
			SizeBytes:   1024,
			CreatedAt:   time.Time{},
		}
		result, err := normalizeArtifactRecord(record, func() time.Time { return now })
		assert.NoError(t, err)
		assert.Equal(t, now.UTC(), result.CreatedAt)
	})

	t.Run("uses record CreatedAt when non-zero", func(t *testing.T) {
		recorded := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
		u, err := uuid.NewV7()
		assert.NoError(t, err)
		record := ArtifactRecord{
			ArtifactID:  u.String(),
			Tenant:      "acme",
			JobID:       "demo",
			RunID:       "run-1",
			Name:        "stdout",
			ContentType: "text/plain",
			SizeBytes:   1024,
			CreatedAt:   recorded,
		}
		result, err := normalizeArtifactRecord(record, nil)
		assert.NoError(t, err)
		assert.Equal(t, recorded.UTC(), result.CreatedAt)
	})
}

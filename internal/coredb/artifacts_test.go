// SPDX-License-Identifier: AGPL-3.0-or-later

package coredb

import (
	"testing"

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

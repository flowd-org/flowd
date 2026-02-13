// SPDX-License-Identifier: AGPL-3.0-or-later

package artifacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/flowd-org/flowd/internal/coredb"
)

const mappingNamespace = "artifacts"

var (
	// ErrMappingStoreUnavailable indicates the mapping store has no DB backend.
	ErrMappingStoreUnavailable = errors.New("artifacts: mapping store unavailable")
	// ErrMappingInvalidScope indicates tenant/job scope fields are missing.
	ErrMappingInvalidScope = errors.New("artifacts: mapping scope invalid")
	// ErrMappingScopeMismatch indicates an artifact_id that does not belong to the provided tenant/job scope.
	ErrMappingScopeMismatch = errors.New("artifacts: mapping scope mismatch")
)

// KeyMappingStore maps logical artifact keys to artifact IDs via Rule-Y KV.
type KeyMappingStore struct {
	kv   *coredb.RuleYStore
	meta *coredb.ArtifactStore
}

// NewKeyMappingStore returns a KV-backed mapping store scoped to a static namespace.
func NewKeyMappingStore(db *coredb.DB) *KeyMappingStore {
	if db == nil {
		return nil
	}
	kv := coredb.NewRuleYStore(db)
	kv.SetAllowlist(map[string]coredb.RuleYNamespaceQuota{
		mappingNamespace: {},
	})
	return &KeyMappingStore{kv: kv, meta: coredb.NewArtifactStore(db)}
}

// Set repoints a logical artifact key to artifactID.
func (s *KeyMappingStore) Set(ctx context.Context, tenant, jobID, artifactKey, artifactID string) error {
	if s == nil || s.kv == nil || s.meta == nil {
		return ErrMappingStoreUnavailable
	}
	normalizedTenant, normalizedJobID, normalizedArtifactKey, err := normalizeMappingScope(tenant, jobID, artifactKey)
	if err != nil {
		return err
	}
	normalizedArtifactID, err := coredb.NormalizeArtifactIDForStorage(artifactID)
	if err != nil {
		return err
	}

	record, found, err := s.meta.Get(ctx, normalizedArtifactID)
	if err != nil {
		return err
	}
	if !found {
		return coredb.ErrArtifactInvalidMetadata
	}
	if !scopeMatches(record, normalizedTenant, normalizedJobID) {
		return ErrMappingScopeMismatch
	}

	key, err := DeriveMappingKey(normalizedTenant, normalizedJobID, normalizedArtifactKey)
	if err != nil {
		return err
	}
	_, err = s.kv.Put(ctx, mappingNamespace, key, []byte(normalizedArtifactID), coredb.RuleYPutOptions{ContentType: "text/plain"})
	return err
}

// Resolve returns the artifact ID currently pointed to by a logical artifact key.
func (s *KeyMappingStore) Resolve(ctx context.Context, tenant, jobID, artifactKey string) (string, bool, error) {
	if s == nil || s.kv == nil || s.meta == nil {
		return "", false, ErrMappingStoreUnavailable
	}
	normalizedTenant, normalizedJobID, normalizedArtifactKey, err := normalizeMappingScope(tenant, jobID, artifactKey)
	if err != nil {
		return "", false, err
	}
	key, err := DeriveMappingKey(normalizedTenant, normalizedJobID, normalizedArtifactKey)
	if err != nil {
		return "", false, err
	}
	result, found, err := s.kv.Get(ctx, mappingNamespace, key)
	if err != nil || !found {
		return "", found, err
	}

	normalizedArtifactID, err := coredb.NormalizeArtifactIDForStorage(string(result.Value))
	if err != nil {
		return "", false, err
	}
	record, exists, err := s.meta.Get(ctx, normalizedArtifactID)
	if err != nil {
		return "", false, err
	}
	if !exists {
		return "", false, nil
	}
	if !scopeMatches(record, normalizedTenant, normalizedJobID) {
		return "", false, ErrMappingScopeMismatch
	}
	return normalizedArtifactID, true, nil
}

// DeriveMappingKey computes the fixed-length Rule-Y KV key for a logical artifact key.
func DeriveMappingKey(tenant, jobID, artifactKey string) (string, error) {
	normalizedTenant, normalizedJobID, normalizedArtifactKey, err := normalizeMappingScope(tenant, jobID, artifactKey)
	if err != nil {
		return "", err
	}
	raw := normalizedTenant + "\n" + normalizedJobID + "\n" + normalizedArtifactKey
	sum := sha256.Sum256([]byte(raw))
	return "ak/" + hex.EncodeToString(sum[:]), nil
}

func normalizeMappingScope(tenant, jobID, artifactKey string) (string, string, string, error) {
	normalizedTenant := strings.ToLower(strings.TrimSpace(tenant))
	normalizedJobID := strings.ToLower(strings.TrimSpace(jobID))
	if normalizedTenant == "" || normalizedJobID == "" {
		return "", "", "", ErrMappingInvalidScope
	}
	normalizedArtifactKey, err := coredb.NormalizeRuleYKey(artifactKey)
	if err != nil {
		return "", "", "", fmt.Errorf("%w: %v", ErrMappingInvalidScope, err)
	}
	return normalizedTenant, normalizedJobID, normalizedArtifactKey, nil
}

func scopeMatches(record coredb.ArtifactRecord, tenant, jobID string) bool {
	return strings.EqualFold(strings.TrimSpace(record.Tenant), tenant) &&
		strings.EqualFold(strings.TrimSpace(record.JobID), jobID)
}

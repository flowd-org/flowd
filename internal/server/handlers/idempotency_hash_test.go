package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestCanonicalJSONHashDeterministic(t *testing.T) {
	bodyA := `{"job_id":"demo","args":{"name":"Alice","count":1,"items":[1,2,{"z":"k","a":"b"}]}}`
	bodyB := `{"args":{"items":[1,2,{"a":"b","z":"k"}],"count":1,"name":"Alice"},"job_id":"demo"}`

	canonicalA, err := canonicalizeJSON([]byte(bodyA))
	if err != nil {
		t.Fatalf("canonicalize bodyA: %v", err)
	}
	canonicalB, err := canonicalizeJSON([]byte(bodyB))
	if err != nil {
		t.Fatalf("canonicalize bodyB: %v", err)
	}

	if string(canonicalA) != string(canonicalB) {
		t.Fatalf("expected canonical JSON to match; got %q vs %q", string(canonicalA), string(canonicalB))
	}

	sumA := sha256.Sum256(canonicalA)
	sumB := sha256.Sum256(canonicalB)
	hashA := hex.EncodeToString(sumA[:])
	hashB := hex.EncodeToString(sumB[:])
	if hashA != hashB {
		t.Fatalf("expected matching hashes; got %s vs %s", hashA, hashB)
	}
	if hashA != strings.ToLower(hashA) {
		t.Fatalf("expected lowercase hex digest, got %s", hashA)
	}
}

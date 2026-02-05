// SPDX-License-Identifier: AGPL-3.0-or-later
package indexer

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

var jobIDSegmentPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

// JobIDError reports invalid job ID segments during normalization.
// Path is the slash-delimited path being normalized and Segment is the raw segment value.
type JobIDError struct {
	Path    string
	Segment string
	Reason  string
}

func (e JobIDError) Error() string {
	if e.Path == "" {
		return fmt.Sprintf("invalid job id segment %q: %s", e.Segment, e.Reason)
	}
	return fmt.Sprintf("invalid job id segment %q in %q: %s", e.Segment, e.Path, e.Reason)
}

type segmentError struct {
	Segment string
	Reason  string
}

func (e segmentError) Error() string {
	return e.Reason
}

// CanonicalJobID returns a canonical slash job ID using mountPath-prefix semantics.
// mountPath "." uses an empty prefix. jobDirRel maps "." to the empty string.
func CanonicalJobID(mountPath, jobDirRel string) (string, error) {
	prefix := strings.TrimSpace(mountPath)
	if prefix == "." {
		prefix = ""
	}

	rel := normalizePathInput(jobDirRel)
	if rel == "." {
		rel = ""
	}

	normalizedPrefix, err := normalizePath(prefix)
	if err != nil {
		return "", err
	}
	normalizedRel, err := normalizePath(rel)
	if err != nil {
		return "", err
	}

	if normalizedPrefix == "" && normalizedRel == "" {
		return "", nil
	}
	if normalizedPrefix == "" {
		return normalizedRel, nil
	}
	if normalizedRel == "" {
		return normalizedPrefix, nil
	}
	return normalizedPrefix + "/" + normalizedRel, nil
}

// DotJobIDToSlash normalizes legacy dot-delimited job IDs into slash-delimited paths.
func DotJobIDToSlash(input string) string {
	trimmed := normalizePathInput(input)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.ReplaceAll(trimmed, ".", "/")
	trimmed = strings.ReplaceAll(trimmed, "//", "/")
	trimmed = strings.Trim(trimmed, "/")
	return trimmed
}

// SlashJobIDToDot derives a dot-delimited identifier from a slash path.
func SlashJobIDToDot(input string) string {
	trimmed := normalizePathInput(input)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.ReplaceAll(trimmed, "//", "/")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return ""
	}
	return strings.ReplaceAll(trimmed, "/", ".")
}

func normalizePathInput(value string) string {
	normalized := strings.TrimSpace(value)
	normalized = strings.ReplaceAll(normalized, "\\", "/")
	return normalized
}

func normalizePath(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	normalized := normalizePathInput(path)
	normalized = strings.Trim(normalized, "/")
	if normalized == "" {
		return "", nil
	}

	segments := strings.Split(normalized, "/")
	out := make([]string, 0, len(segments))
	for _, seg := range segments {
		if seg == "" {
			return "", JobIDError{Path: normalized, Segment: seg, Reason: "empty segment"}
		}
		normalizedSegment, err := normalizeSegment(seg)
		if err != nil {
			reason := err.Error()
			return "", JobIDError{Path: normalized, Segment: seg, Reason: reason}
		}
		out = append(out, normalizedSegment)
	}
	return strings.Join(out, "/"), nil
}

func normalizeSegment(segment string) (string, error) {
	trimmed := strings.TrimSpace(segment)
	if trimmed == "" {
		return "", segmentError{Segment: segment, Reason: "empty segment"}
	}

	var b strings.Builder
	b.Grow(len(trimmed))
	lastDash := false
	for _, r := range trimmed {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		} else if b.Len() == 0 {
			lastDash = true
		}
	}

	normalized := b.String()
	normalized = strings.TrimSuffix(normalized, "-")
	if normalized == "" {
		return "", segmentError{Segment: segment, Reason: "segment normalizes to empty"}
	}
	if !jobIDSegmentPattern.MatchString(normalized) {
		return "", segmentError{Segment: segment, Reason: "segment does not match required pattern"}
	}
	return normalized, nil
}

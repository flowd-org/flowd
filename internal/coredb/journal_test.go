package coredb

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestJournalAppendAndIterate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dir := t.TempDir()
	db, err := Open(ctx, Options{DataDir: dir})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	journal := NewJournal(db, 0)

	ts := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
	first, err := journal.Append(ctx, "run-1", "run.start", []byte(`{"status":"running"}`), ts)
	if err != nil {
		t.Fatalf("append first: %v", err)
	}
	if first.Seq == 0 {
		t.Fatalf("expected sequence > 0")
	}

	second, err := journal.Append(ctx, "run-1", "step.log", []byte(`{"message":"hello"}`), ts.Add(time.Second))
	if err != nil {
		t.Fatalf("append second: %v", err)
	}
	if second.Seq <= first.Seq {
		t.Fatalf("expected second seq greater than first (first=%d second=%d)", first.Seq, second.Seq)
	}

	var entries []JournalEntry
	if err := journal.ForEach(ctx, "run-1", 0, func(e JournalEntry) error {
		entries = append(entries, e)
		return nil
	}); err != nil {
		t.Fatalf("journal iterate: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Seq != first.Seq || entries[1].Seq != second.Seq {
		t.Fatalf("unexpected sequences: %#v", entries)
	}
	if !entries[0].Timestamp.Equal(ts) {
		t.Fatalf("expected first timestamp %v, got %v", ts, entries[0].Timestamp)
	}
	if entries[1].EventType != "step.log" {
		t.Fatalf("expected event type step.log, got %s", entries[1].EventType)
	}
}

func TestJournalEvictsOldestWhenOverLimit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dir := t.TempDir()
	db, err := Open(ctx, Options{DataDir: dir})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	// Limit well below two payloads to force eviction of the first.
	journal := NewJournal(db, 30)

	if _, err := journal.Append(ctx, "run-1", "step.log", []byte(`{"message":"alpha"}`), time.Now().UTC()); err != nil {
		t.Fatalf("append alpha: %v", err)
	}
	second, err := journal.Append(ctx, "run-1", "step.log", []byte(`{"message":"bravo"}`), time.Now().UTC())
	if err != nil {
		t.Fatalf("append bravo: %v", err)
	}

	var sequences []int64
	if err := journal.ForEach(ctx, "run-1", 0, func(e JournalEntry) error {
		sequences = append(sequences, e.Seq)
		return nil
	}); err != nil {
		t.Fatalf("iterate: %v", err)
	}
	if len(sequences) != 1 {
		t.Fatalf("expected single retained entry, got %d", len(sequences))
	}
	if sequences[0] != second.Seq {
		t.Fatalf("expected retained seq %d, got %d", second.Seq, sequences[0])
	}

	earliest, latest, err := journal.Bounds(ctx, "run-1")
	if err != nil {
		t.Fatalf("bounds: %v", err)
	}
	if earliest != second.Seq || latest != second.Seq {
		t.Fatalf("expected bounds to equal second seq %d, got earliest=%d latest=%d", second.Seq, earliest, latest)
	}
}

func TestJournalRejectsPayloadAboveLimit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dir := t.TempDir()
	db, err := Open(ctx, Options{DataDir: dir})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	journal := NewJournal(db, 8) // eight bytes max
	_, err = journal.Append(ctx, "run-1", "step.log", []byte(`{"msg":"too big"}`), time.Now().UTC())
	if !errors.Is(err, ErrJournalQuotaExceeded) {
		t.Fatalf("expected ErrJournalQuotaExceeded, got %v", err)
	}
}

func TestParseEventID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		wantSeq int64
		wantErr bool
	}{
		{"empty", "", 0, false},
		{"spaces", " 42 ", 42, false},
		{"invalid", "abc", 0, true},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			seq, err := ParseEventID(tc.input)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if seq != tc.wantSeq {
				t.Fatalf("expected %d, got %d", tc.wantSeq, seq)
			}
		})
	}
}

package events

import (
	"bytes"
	"testing"
)

func TestRedactSecrets(t *testing.T) {
	vals := map[string]interface{}{"a": "1", "secret": "value"}
	secrets := map[string]struct{}{"secret": {}}
	redacted := RedactSecrets(vals, secrets)
	if redacted["secret"] != "$$REDACTED$$" {
		t.Fatalf("expected secret redacted, got %v", redacted["secret"])
	}
	if redacted["a"] != "1" {
		t.Fatalf("expected non secret preserved")
	}
}

func TestSecretTokenValue(t *testing.T) {
	if SecretToken() != "$$REDACTED$$" {
		t.Fatalf("expected $$REDACTED$$ token, got %s", SecretToken())
	}
}

func TestNewLineRedactor(t *testing.T) {
	redactor := NewLineRedactor([]string{"token"})
	if redactor == nil {
		t.Fatalf("expected redactor")
	}
	line := redactor("value token here")
	if line != "value $$REDACTED$$ here" {
		t.Fatalf("expected redaction, got %s", line)
	}
	if NewLineRedactor(nil) != nil {
		t.Fatalf("expected nil redactor for empty secrets")
	}
}

type fakeSink struct {
	calls map[string]int
}

func (f *fakeSink) EmitRunStart(runID, jobID string) {
	if f.calls == nil {
		f.calls = make(map[string]int)
	}
	f.calls["EmitRunStart"]++
}

func (f *fakeSink) EmitRunFinish(runID, status string, err error) {
	if f.calls == nil {
		f.calls = make(map[string]int)
	}
	f.calls["EmitRunFinish"]++
}

func (f *fakeSink) EmitStepStart(runID, step string) {
	if f.calls == nil {
		f.calls = make(map[string]int)
	}
	f.calls["EmitStepStart"]++
}

func (f *fakeSink) EmitStepLog(runID, step, channel, message string) {
	if f.calls == nil {
		f.calls = make(map[string]int)
	}
	f.calls["EmitStepLog"]++
}

func (f *fakeSink) EmitStepFinish(runID, step string, exitCode int, err error) {
	if f.calls == nil {
		f.calls = make(map[string]int)
	}
	f.calls["EmitStepFinish"]++
}

func TestNewCompositeSink_NilSinks(t *testing.T) {
	sink := NewCompositeSink(nil, nil)
	if sink != nil {
		t.Fatalf("expected nil for all nil sinks")
	}
}

func TestNewCompositeSink_SingleNil(t *testing.T) {
	sink := NewCompositeSink(nil)
	if sink != nil {
		t.Fatalf("expected nil for single nil sink")
	}
}

func TestNewCompositeSink_OneSink(t *testing.T) {
	fake := &fakeSink{}
	sink := NewCompositeSink(fake)
	if sink != fake {
		t.Fatalf("expected same sink returned for single non-nil sink")
	}
}

func TestNewCompositeSink_MultipleSinks(t *testing.T) {
	f1 := &fakeSink{}
	f2 := &fakeSink{}
	sink := NewCompositeSink(f1, f2)

	cs, ok := sink.(*CompositeSink)
	if !ok {
		t.Fatalf("expected CompositeSink for multiple sinks")
	}

	if len(cs.sinks) != 2 {
		t.Fatalf("expected 2 sinks, got %d", len(cs.sinks))
	}

	sink.EmitRunStart("run1", "job1")
	if f1.calls["EmitRunStart"] != 1 || f2.calls["EmitRunStart"] != 1 {
		t.Fatalf("expected both sinks to receive EmitRunStart")
	}
}

func TestNewCompositeSink_EmitAllMethods(t *testing.T) {
	fake := &fakeSink{}
	sink := NewCompositeSink(fake)

	sink.EmitRunStart("run1", "job1")
	sink.EmitRunFinish("run1", "success", nil)
	sink.EmitStepStart("run1", "step1")
	sink.EmitStepLog("run1", "step1", "stdout", "hello")
	sink.EmitStepFinish("run1", "step1", 0, nil)

	expectedCalls := map[string]int{
		"EmitRunStart":   1,
		"EmitRunFinish":  1,
		"EmitStepStart":  1,
		"EmitStepLog":    1,
		"EmitStepFinish": 1,
	}

	for method, count := range expectedCalls {
		if fake.calls[method] != count {
			t.Fatalf("expected %s called %d times, got %d", method, count, fake.calls[method])
		}
	}
}

func TestNewStepWriter(t *testing.T) {
	var buf bytes.Buffer
	redactor := func(s string) string { return s }
	w := NewStepWriter(nil, "run1", "step1", "stdout", &buf, redactor)

	if w == nil {
		t.Fatalf("expected StepWriter")
	}
	if w.runID != "run1" || w.stepID != "step1" || w.channel != "stdout" {
		t.Fatalf("unexpected fields")
	}
}

func TestStepWriter_Write(t *testing.T) {
	var buf bytes.Buffer
	w := NewStepWriter(nil, "run1", "step1", "stdout", &buf, nil)

	n, err := w.Write([]byte("hello\nworld\n"))
	if err != nil || n != 12 {
		t.Fatalf("Write failed: %d %v", n, err)
	}
}

func TestStepWriter_WriteEmpty(t *testing.T) {
	var buf bytes.Buffer
	w := NewStepWriter(nil, "run1", "step1", "stdout", &buf, nil)

	n, err := w.Write([]byte{})
	if err != nil || n != 0 {
		t.Fatalf("Write empty failed: %d %v", n, err)
	}
}

func TestStepWriter_WriteWithOutputError(t *testing.T) {
	w := NewStepWriter(nil, "run1", "step1", "stdout", nil, nil)

	n, err := w.Write([]byte("test"))
	if err != nil || n != 4 {
		t.Fatalf("Write without output failed: %d %v", n, err)
	}
}

func TestStepWriter_Flush(t *testing.T) {
	var buf bytes.Buffer
	fake := &fakeSink{}
	w := NewStepWriter(fake, "run1", "step1", "stdout", &buf, nil)

	w.Write([]byte("line1\n"))
	w.Flush()

	if fake.calls["EmitStepLog"] != 1 {
		t.Fatalf("expected EmitStepLog called once after Flush")
	}
}

func TestStepWriter_FlushEmpty(t *testing.T) {
	var buf bytes.Buffer
	fake := &fakeSink{}
	w := NewStepWriter(fake, "run1", "step1", "stdout", &buf, nil)

	w.Flush()

	if fake.calls["EmitStepLog"] != 0 {
		t.Fatalf("expected no EmitStepLog for empty flush")
	}
}

func TestNewJournalSink_Nil(t *testing.T) {
	sink := NewJournalSink(nil)
	if sink != nil {
		t.Fatalf("expected nil JournalSink for nil journal")
	}
}

func TestJournalSink_EmitStepLog_EmptyMessage(t *testing.T) {
	sink := &JournalSink{}

	sink.EmitStepLog("run1", "step1", "stdout", "")
}

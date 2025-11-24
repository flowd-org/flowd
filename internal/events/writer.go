// SPDX-License-Identifier: AGPL-3.0-or-later
package events

import (
	"bytes"
	"io"
)

type StepWriter struct {
	emitter  Sink
	runID    string
	stepID   string
	channel  string
	out      io.Writer
	buf      bytes.Buffer
	redactor func(string) string
}

func NewStepWriter(em Sink, runID, stepID, channel string, out io.Writer, redactor func(string) string) *StepWriter {
	return &StepWriter{emitter: em, runID: runID, stepID: stepID, channel: channel, out: out, redactor: redactor}
}

func (w *StepWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if w.out != nil {
		if _, err := w.out.Write(p); err != nil {
			return 0, err
		}
	}
	start := 0
	for i, b := range p {
		if b == '\n' {
			w.buf.Write(p[start:i])
			w.flushLine()
			start = i + 1
		}
	}
	if start < len(p) {
		w.buf.Write(p[start:])
	}
	return len(p), nil
}

func (w *StepWriter) Flush() {
	if w.buf.Len() > 0 {
		w.flushLine()
	}
}

func (w *StepWriter) flushLine() {
	line := w.buf.String()
	w.buf.Reset()
	if w.emitter != nil {
		if w.redactor != nil {
			line = w.redactor(line)
		}
		w.emitter.EmitStepLog(w.runID, w.stepID, w.channel, line)
	}
}

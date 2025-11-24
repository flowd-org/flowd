// SPDX-License-Identifier: AGPL-3.0-or-later
package events

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

const (
	TypeRunStart   = "run.start"
	TypeRunFinish  = "run.finish"
	TypeStepStart  = "step.start"
	TypeStepLog    = "step.log"
	TypeStepFinish = "step.finish"
)

type RunEvent struct {
	Sequence  int64                  `json:"sequence"`
	Timestamp time.Time              `json:"timestamp"`
	Type      string                 `json:"type"`
	RunID     string                 `json:"run_id"`
	Step      string                 `json:"step,omitempty"`
	Channel   string                 `json:"channel,omitempty"`
	Message   string                 `json:"message,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

type Emitter struct {
	mu   sync.Mutex
	seq  int64
	out  io.Writer
	json bool
}

func NewEmitter(out io.Writer, json bool) *Emitter {
	if out == nil {
		return nil
	}
	return &Emitter{out: out, json: json}
}

func (e *Emitter) nextSeq() int64 {
	e.seq++
	return e.seq
}

func (e *Emitter) emit(ev RunEvent) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	ev.Sequence = e.nextSeq()
	ev.Timestamp = time.Now().UTC()

	if e.json {
		payload, err := json.Marshal(ev)
		if err != nil {
			fmt.Fprintf(e.out, "{\"error\":%q}\n", err.Error())
			return
		}
		fmt.Fprintf(e.out, "%s\n", payload)
		return
	}

	fmt.Fprintf(e.out, "[%d] %s", ev.Sequence, ev.Type)
	if ev.RunID != "" {
		fmt.Fprintf(e.out, " run=%s", ev.RunID)
	}
	if ev.Step != "" {
		fmt.Fprintf(e.out, " step=%s", ev.Step)
	}
	if ev.Channel != "" {
		fmt.Fprintf(e.out, " channel=%s", ev.Channel)
	}
	if ev.Message != "" {
		fmt.Fprintf(e.out, " msg=%s", ev.Message)
	}
	if len(ev.Data) > 0 {
		first := true
		fmt.Fprintf(e.out, " data=")
		fmt.Fprintf(e.out, "{")
		for k, v := range ev.Data {
			if !first {
				fmt.Fprintf(e.out, ", ")
			}
			fmt.Fprintf(e.out, "%s:%v", k, v)
			first = false
		}
		fmt.Fprintf(e.out, "}")
	}
	fmt.Fprintln(e.out)
}

func (e *Emitter) EmitRunStart(runID, jobID string) {
	e.emit(RunEvent{
		Type:  TypeRunStart,
		RunID: runID,
		Data:  map[string]interface{}{"job_id": jobID},
	})
}

func (e *Emitter) EmitRunFinish(runID string, status string, err error) {
	data := map[string]interface{}{"status": status}
	if err != nil {
		data["error"] = err.Error()
	}
	e.emit(RunEvent{
		Type:  TypeRunFinish,
		RunID: runID,
		Data:  data,
	})
}

func (e *Emitter) EmitStepStart(runID, step string) {
	e.emit(RunEvent{Type: TypeStepStart, RunID: runID, Step: step})
}

func (e *Emitter) EmitStepLog(runID, step, channel, message string) {
	if message == "" {
		return
	}
	e.emit(RunEvent{Type: TypeStepLog, RunID: runID, Step: step, Channel: channel, Message: message})
}

func (e *Emitter) EmitStepFinish(runID, step string, exitCode int, err error) {
	status := "completed"
	if exitCode != 0 || err != nil {
		status = "failed"
	}
	data := map[string]interface{}{"exit_code": exitCode, "status": status}
	if err != nil {
		data["error"] = err.Error()
	}
	e.emit(RunEvent{Type: TypeStepFinish, RunID: runID, Step: step, Data: data})
}

func GenerateRunID() string {
	return fmt.Sprintf("run-%d", time.Now().UnixNano())
}

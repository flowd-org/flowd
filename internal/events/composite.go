package events

// Sink represents something that can consume run events.
type Sink interface {
	EmitRunStart(runID, jobID string)
	EmitRunFinish(runID, status string, err error)
	EmitStepStart(runID, step string)
	EmitStepLog(runID, step, channel, message string)
	EmitStepFinish(runID, step string, exitCode int, err error)
}

// CompositeSink fan-outs emitted events to multiple sinks.
type CompositeSink struct {
	sinks []Sink
}

// NewCompositeSink returns a sink that forwards events to all provided sinks.
func NewCompositeSink(sinks ...Sink) Sink {
	filtered := make([]Sink, 0, len(sinks))
	for _, s := range sinks {
		if s != nil {
			filtered = append(filtered, s)
		}
	}
	switch len(filtered) {
	case 0:
		return nil
	case 1:
		return filtered[0]
	default:
		return &CompositeSink{sinks: filtered}
	}
}

func (c *CompositeSink) EmitRunStart(runID, jobID string) {
	for _, s := range c.sinks {
		s.EmitRunStart(runID, jobID)
	}
}

func (c *CompositeSink) EmitRunFinish(runID, status string, err error) {
	for _, s := range c.sinks {
		s.EmitRunFinish(runID, status, err)
	}
}

func (c *CompositeSink) EmitStepStart(runID, step string) {
	for _, s := range c.sinks {
		s.EmitStepStart(runID, step)
	}
}

func (c *CompositeSink) EmitStepLog(runID, step, channel, message string) {
	for _, s := range c.sinks {
		s.EmitStepLog(runID, step, channel, message)
	}
}

func (c *CompositeSink) EmitStepFinish(runID, step string, exitCode int, err error) {
	for _, s := range c.sinks {
		s.EmitStepFinish(runID, step, exitCode, err)
	}
}

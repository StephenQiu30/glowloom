package application

const (
	DefaultMaxTurnModelSteps = 40
	DefaultMaxTurnToolSteps  = 40
)

// TurnBudgetTracker enforces single-turn model request / tool step ceilings and manages fee reservations.
type TurnBudgetTracker struct {
	MaxModelSteps int
	MaxToolSteps  int
	ModelSteps    int
	ToolSteps     int

	ReservedMicros int64
	ConsumedMicros int64
}

// NewTurnBudgetTracker initializes budget tracking with default or custom limits.
func NewTurnBudgetTracker(maxModelSteps, maxToolSteps int, reservedMicros int64) *TurnBudgetTracker {
	if maxModelSteps <= 0 {
		maxModelSteps = DefaultMaxTurnModelSteps
	}
	if maxToolSteps <= 0 {
		maxToolSteps = DefaultMaxTurnToolSteps
	}
	return &TurnBudgetTracker{
		MaxModelSteps:  maxModelSteps,
		MaxToolSteps:   maxToolSteps,
		ReservedMicros: reservedMicros,
	}
}

// RecordModelStep increments the model invocation step count and validates cost against reservation.
func (t *TurnBudgetTracker) RecordModelStep(costMicros int64) error {
	if t.ModelSteps >= t.MaxModelSteps {
		return ErrTurnBudgetExceeded
	}
	t.ModelSteps++
	return t.recordCost(costMicros)
}

// RecordToolStep increments the tool invocation step count and validates cost against reservation.
func (t *TurnBudgetTracker) RecordToolStep(costMicros int64) error {
	if t.ToolSteps >= t.MaxToolSteps {
		return ErrTurnBudgetExceeded
	}
	t.ToolSteps++
	return t.recordCost(costMicros)
}

func (t *TurnBudgetTracker) recordCost(micros int64) error {
	if micros <= 0 {
		return nil
	}
	if t.ReservedMicros > 0 && t.ConsumedMicros+micros > t.ReservedMicros {
		return ErrBudgetOverdraft
	}
	t.ConsumedMicros += micros
	return nil
}

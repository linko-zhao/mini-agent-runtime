package runstate

import (
	"context"
	"errors"
	"reflect"
)

var (
	ErrOutcomeIncomplete = errors.New("runstate: not all tool results are known")
	ErrOutcomeConflict   = errors.New("runstate: run changed during outcome inspection")
)

// ReconcileToolResults repairs missing tool messages on THIS live Run only.
// Queries occur outside the gate lock. The original observation is compared
// and ToolsReturned committed under one gate lock, preventing duplicate commits
// through this unique gate. It never executes/retries business tools or restores
// a process. Every return includes an owned current State; callers must check err.
func (g *PauseGate) ReconcileToolResults(queryCtx context.Context, source ToolOutcomeSource) (State, error) {
	observed := g.Snapshot()
	report, err := InspectToolOutcomes(queryCtx, source, observed)
	if err != nil {
		return g.Snapshot(), err
	}
	if !report.AllResultsKnown {
		return g.Snapshot(), ErrOutcomeIncomplete
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	current := g.run.Snapshot()
	if err := queryCtx.Err(); err != nil {
		return current, err
	}
	if !reflect.DeepEqual(current, report.Observed) {
		return current, ErrOutcomeConflict
	}
	// Run.Apply observes the ORIGINAL execution ctx. A fresh query ctx cannot
	// revive a cancelled execution, and a late query cancellation cannot roll back
	// a successful commit. Do not recursively call Gate.Apply while holding g.mu.
	state, err := g.run.Apply(Event{Kind: ToolsReturned, Results: report.Results})
	if terminal(state.Phase) {
		g.request, g.decision = nil, nil
	}
	return state, err
}

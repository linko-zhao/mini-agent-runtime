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

// ReconcileToolResults queries outside the lock and conditionally commits once.
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
	s := g.Snapshot()
	if g.run.ctx.Err() != nil {
		next, transitionErr := Advance(s, Event{Kind: Cancel})
		if transitionErr != nil {
			return s, transitionErr
		}
		return next, g.run.ctx.Err()
	}
	if !reflect.DeepEqual(observed, s) {
		return s, ErrOutcomeConflict
	}
	next, err := g.run.Apply(Event{Kind: ToolsReturned, Results: report.Results})
	if err != nil {
		return s, err
	}
	if terminal(next.Phase) {
		g.request = nil
		g.desicion = nil
	}
	if g.run.ctx.Err() != nil {
		n, transitionErr := Advance(s, Event{Kind: Cancel})
		if transitionErr != nil {
			return s, transitionErr
		}
		return n, err
	}
	return next, nil
}

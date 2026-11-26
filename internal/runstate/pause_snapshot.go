package runstate

import (
	"bytes"
	"encoding/gob"
	"errors"
	"strings"
)

const (
	PauseSnapshotSchema   = 1
	MaxPauseSnapshotBytes = 1 << 20
)

var ErrInvalidPauseSnapshot = errors.New("runstate: invalid pause snapshot")

type PauseSnapshot struct {
	SchemaVersion int
	Request       PauseRequest
	Decision      *ApprovalDecision
}

func (s PauseSnapshot) Clone() PauseSnapshot {
	return PauseSnapshot{
		SchemaVersion: s.SchemaVersion,
		Request:       PauseRequest{ID: s.Request.ID, Reason: s.Request.Reason, Checkpoint: s.Request.Checkpoint.Clone()},
		Decision:      s.Decision,
	}
}
func validatePauseSnapshot(ps PauseSnapshot) bool {
	if ps.SchemaVersion != PauseSnapshotSchema {
		return false
	}
	p := ps.Request
	if !validateCheckpoint(p.Checkpoint) || p.Reason == "" {
		return false
	}
	s := p.Checkpoint.State
	if p.Checkpoint.Boundary != CheckpointBeforeTools || s.Result != "" || s.Failure != "" || len(s.Messages) == 0 || len(s.Pending) == 0 {
		return false
	}
	msgs := s.Messages
	first := msgs[0]
	last := msgs[len(msgs)-1]
	if first.Role != "user" || last.Role != "assistant" || len(last.ToolCalls) != len(s.Pending) {
		return false
	}
	pending := s.Pending
	ids := map[string]bool{}
	for i, c := range last.ToolCalls {
		if c.ID != pending[i].ID || c.Name != pending[i].Name || !bytes.Equal(c.Arguments, pending[i].Arguments) || ids[c.ID] {
			return false
		}
		ids[c.ID] = true
	}
	if d := ps.Decision; d != nil {
		if d.RunID != p.Checkpoint.State.RunID || d.RequestID != p.ID || strings.TrimSpace(d.Actor) == "" || strings.TrimSpace(d.Reason) == "" || (d.Outcome != DecisionApprove && d.Outcome != DecisionReject) {
			return false
		}
	}
	return true
}
func (g *PauseGate) PauseSnapshot() (PauseSnapshot, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.run.ctx.Err(); err != nil {
		return PauseSnapshot{}, err
	}
	if g.request == nil {
		return PauseSnapshot{}, ErrNoPause
	}
	ps := PauseSnapshot{
		SchemaVersion: PauseSnapshotSchema,
		Request:       PauseRequest{ID: g.request.ID, Reason: g.request.Reason, Checkpoint: g.request.Checkpoint.Clone()},
		Decision:      g.desicion,
	}
	if !validatePauseSnapshot(ps) {
		return PauseSnapshot{}, ErrInvalidPauseSnapshot
	}
	if err := g.run.ctx.Err(); err != nil {
		return PauseSnapshot{}, err
	}
	return ps, nil
}
func EncodePauseSnapshot(s PauseSnapshot) ([]byte, error) {
	if !validatePauseSnapshot(s) {
		return nil, ErrInvalidPauseSnapshot
	}
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(s); err != nil {
		return nil, ErrInvalidPauseSnapshot
	}
	if buf.Len() > MaxPauseSnapshotBytes {
		return nil, ErrInvalidPauseSnapshot
	}
	return buf.Bytes(), nil
}
func DecodePauseSnapshot(data []byte) (PauseSnapshot, error) {
	if data == nil || len(data) > MaxPauseSnapshotBytes {
		return PauseSnapshot{}, ErrInvalidPauseSnapshot
	}
	buf := bytes.NewReader(data)
	dec := gob.NewDecoder(buf)
	var s PauseSnapshot
	if err := dec.Decode(&s); err != nil || buf.Len() != 0 || !validatePauseSnapshot(s) {
		return PauseSnapshot{}, ErrInvalidPauseSnapshot
	}
	return s, nil
}

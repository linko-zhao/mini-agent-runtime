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

// PauseSnapshot is a versioned host-owned candidate, not a saved receipt,
// restored Gate, fresh-state proof, approval credential or execution lease.
type PauseSnapshot struct {
	SchemaVersion int
	Request       PauseRequest
	Decision      *ApprovalDecision
}

// Clone copies mutable messages, calls and the optional decision.
// It does not validate, save or authorize this value.
func (s PauseSnapshot) Clone() PauseSnapshot {
	s.Request.Checkpoint = s.Request.Checkpoint.Clone()
	if s.Decision != nil {
		d := *s.Decision
		s.Decision = &d
	}
	return s
}

// PauseSnapshot captures held request and decision under the same gate lock.
// It observes cancellation without committing it; it never calls Apply or I/O.
func (g *PauseGate) PauseSnapshot() (PauseSnapshot, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.run.Context().Err(); err != nil {
		return PauseSnapshot{}, err
	}
	if g.request == nil {
		return PauseSnapshot{}, ErrNoPause
	}
	s := PauseSnapshot{SchemaVersion: PauseSnapshotSchema, Request: *g.request, Decision: g.decision}.Clone()
	if !pauseSnapshotValid(s) {
		return PauseSnapshot{}, ErrInvalidPauseSnapshot
	}
	if err := g.run.Context().Err(); err != nil {
		return PauseSnapshot{}, err
	}
	return s, nil
}

// Limited consistency checks, not full history/provenance/authorization checks.
func pauseSnapshotValid(s PauseSnapshot) bool {
	p, cp := s.Request, s.Request.Checkpoint
	st := cp.State
	if s.SchemaVersion != PauseSnapshotSchema || strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.Reason) == "" ||
		!validateCandidate(cp) || cp.Boundary != CheckpointBeforeTools || st.Phase != ToolsPending ||
		st.Result != "" || st.Failure != "" || len(st.Messages) == 0 || st.Messages[0].Role != "user" || len(st.Pending) == 0 {
		return false
	}
	last := st.Messages[len(st.Messages)-1]
	if last.Role != "assistant" || len(last.ToolCalls) != len(st.Pending) {
		return false
	}
	ids := map[string]bool{}
	for i, call := range st.Pending {
		h := last.ToolCalls[i]
		if call.ID == "" || strings.TrimSpace(call.Name) == "" || ids[call.ID] || call.ID != h.ID || call.Name != h.Name || !bytes.Equal(call.Arguments, h.Arguments) {
			return false
		}
		ids[call.ID] = true
	}
	if d := s.Decision; d != nil {
		if d.RunID != st.RunID || d.RequestID != p.ID || strings.TrimSpace(d.Actor) == "" || strings.TrimSpace(d.Reason) == "" ||
			(d.Outcome != DecisionApprove && d.Outcome != DecisionReject) {
			return false
		}
	}
	return true
}

// EncodePauseSnapshot uses a fresh gob stream and preserves argument bytes.
// No I/O/ctx is involved. This is not canonical encoding or a storage commit.
func EncodePauseSnapshot(s PauseSnapshot) ([]byte, error) {
	if !pauseSnapshotValid(s) {
		return nil, ErrInvalidPauseSnapshot
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(s); err != nil || buf.Len() > MaxPauseSnapshotBytes {
		return nil, ErrInvalidPauseSnapshot
	}
	return buf.Bytes(), nil
}

// DecodePauseSnapshot accepts one bounded host artifact, not arbitrary network
// input. Gob type/field handling remains the standard library's compatibility
// behavior; this is not an authenticated or hostile-input-safe file format.
func DecodePauseSnapshot(data []byte) (PauseSnapshot, error) {
	if len(data) == 0 || len(data) > MaxPauseSnapshotBytes {
		return PauseSnapshot{}, ErrInvalidPauseSnapshot
	}
	reader := bytes.NewReader(data)
	var out PauseSnapshot
	if err := gob.NewDecoder(reader).Decode(&out); err != nil || reader.Len() != 0 || !pauseSnapshotValid(out) {
		return PauseSnapshot{}, ErrInvalidPauseSnapshot
	}
	return out, nil
}

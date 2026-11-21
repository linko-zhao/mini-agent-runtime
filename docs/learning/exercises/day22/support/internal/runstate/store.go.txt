package runstate

import (
	"context"
	"errors"
	"strings"
	"sync"
)

var (
	ErrCheckpointNotFound = errors.New("runstate: checkpoint not found")
	ErrCheckpointConflict = errors.New("runstate: checkpoint version conflict")
	ErrInvalidCheckpoint  = errors.New("runstate: invalid checkpoint request")
)

// StoredCheckpoint is the latest saved value. Version is a per-RunID CAS token,
// not State.Step, a schema version, an execution lease or a resume permission.
type StoredCheckpoint struct {
	Version    uint64
	Checkpoint Checkpoint
}

type CheckpointStore interface {
	Load(context.Context, string) (StoredCheckpoint, error)
	Save(context.Context, Checkpoint, uint64) (StoredCheckpoint, error)
}

// MemoryCheckpointStore is process-local and not durable. Its zero value works.
// Do not copy after use. It retains only the latest record for each exact RunID.
type MemoryCheckpointStore struct {
	mu      sync.Mutex
	records map[string]StoredCheckpoint
}

func NewMemoryCheckpointStore() *MemoryCheckpointStore { return &MemoryCheckpointStore{} }

func cloneStored(v StoredCheckpoint) StoredCheckpoint {
	v.Checkpoint = v.Checkpoint.Clone()
	return v
}
func storeContext(ctx context.Context) error {
	if ctx == nil {
		return ErrInvalidCheckpoint
	}
	return ctx.Err()
}

// validateCandidate checks only the trusted candidate envelope, not full message
// history, authorization, authenticity or an untrusted serialized payload.
func validateCandidate(cp Checkpoint) bool {
	s := cp.State
	if strings.TrimSpace(s.RunID) == "" || s.MaxSteps <= 0 || s.Step < 0 || s.Step > s.MaxSteps {
		return false
	}
	switch cp.Boundary {
	case CheckpointReady:
		return s.Phase == Ready
	case CheckpointBeforeTools:
		return s.Phase == ToolsPending
	case CheckpointTerminal:
		return terminal(s.Phase)
	default:
		return false
	}
}

func (s *MemoryCheckpointStore) Load(ctx context.Context, runID string) (StoredCheckpoint, error) {
	if err := storeContext(ctx); err != nil {
		return StoredCheckpoint{}, err
	}
	if strings.TrimSpace(runID) == "" {
		return StoredCheckpoint{}, ErrInvalidCheckpoint
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return StoredCheckpoint{}, err
	}
	v, ok := s.records[runID]
	if !ok {
		return StoredCheckpoint{}, ErrCheckpointNotFound
	}
	return cloneStored(v), nil
}

// Save: expected=0 creates only if absent; otherwise it must equal current Version.
// A successful write increments Version even for identical content.
// Errors in this in-memory implementation return zero and leave stored data intact.
// Cancellation is sampled before and after locking; later cancellation can still
// accompany a committed success. The lock wait itself is not context-interruptible.
func (s *MemoryCheckpointStore) Save(ctx context.Context, cp Checkpoint, expected uint64) (StoredCheckpoint, error) {
	if err := storeContext(ctx); err != nil {
		return StoredCheckpoint{}, err
	}
	if expected == ^uint64(0) || !validateCandidate(cp) {
		return StoredCheckpoint{}, ErrInvalidCheckpoint
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return StoredCheckpoint{}, err
	}
	current := s.records[cp.State.RunID]
	if current.Version != expected {
		return StoredCheckpoint{}, ErrCheckpointConflict
	}
	next := StoredCheckpoint{Version: expected + 1, Checkpoint: cp.Clone()}
	if s.records == nil {
		s.records = make(map[string]StoredCheckpoint)
	}
	s.records[cp.State.RunID] = next
	return cloneStored(next), nil
}

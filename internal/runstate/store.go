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

type StoredCheckpoint struct {
	Version    uint64
	Checkpoint Checkpoint
}
type CheckpointStore interface {
	Load(context.Context, string) (StoredCheckpoint, error)
	Save(context.Context, Checkpoint, uint64) (StoredCheckpoint, error)
}
type MemoryCheckpointStore struct {
	mu      sync.Mutex
	records map[string]StoredCheckpoint
}

func storeContext(ctx context.Context) error {
	if ctx == nil {
		return ErrInvalidCheckpoint
	}
	return ctx.Err()
}

func cloneStore(sc StoredCheckpoint) StoredCheckpoint {
	sc.Checkpoint = sc.Checkpoint.Clone()
	return sc
}

func validateCheckpoint(cp Checkpoint) bool {
	if strings.TrimSpace(cp.State.RunID) == "" || cp.State.MaxSteps <= 0 || cp.State.Step < 0 || cp.State.Step > cp.State.MaxSteps {
		return false
	}
	switch cp.Boundary {
	case CheckpointReady:
		return cp.State.Phase == Ready
	case CheckpointBeforeTools:
		return cp.State.Phase == ToolsPending
	case CheckpointTerminal:
		return terminal(cp.State.Phase)
	default:
		return false
	}
}
func NewMemoryCheckpointStore() *MemoryCheckpointStore {
	return &MemoryCheckpointStore{records: make(map[string]StoredCheckpoint)}
}
func (s *MemoryCheckpointStore) Load(ctx context.Context, runID string) (StoredCheckpoint, error) {
	if err := storeContext(ctx); err != nil {
		return StoredCheckpoint{}, err
	}
	if strings.TrimSpace(runID) == "" {
		return StoredCheckpoint{}, ErrInvalidCheckpoint
	}
	if s.records == nil {
		return StoredCheckpoint{}, ErrCheckpointNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		return StoredCheckpoint{}, ctx.Err()
	}
	sc, ok := s.records[runID]
	if !ok {
		return StoredCheckpoint{}, ErrCheckpointNotFound
	}
	return cloneStore(sc), nil
}
func (s *MemoryCheckpointStore) Save(ctx context.Context, cp Checkpoint, expected uint64) (StoredCheckpoint, error) {
	if err := storeContext(ctx); err != nil {
		return StoredCheckpoint{}, err
	}
	if !validateCheckpoint(cp) || expected == ^uint64(0) {
		return StoredCheckpoint{}, ErrInvalidCheckpoint
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		return StoredCheckpoint{}, ctx.Err()
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
	return cloneStore(next), nil
}

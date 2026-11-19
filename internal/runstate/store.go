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
	if ctx == nil {
		return StoredCheckpoint{}, ErrInvalidCheckpoint
	}
	if ctx.Err() != nil {
		return StoredCheckpoint{}, ctx.Err()
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
	return StoredCheckpoint{Version: sc.Version, Checkpoint: sc.Checkpoint.Clone()}, nil
}
func (s *MemoryCheckpointStore) Save(ctx context.Context, cp Checkpoint, expected uint64) (StoredCheckpoint, error) {
	if ctx == nil {
		return StoredCheckpoint{}, ErrInvalidCheckpoint
	}
	if ctx.Err() != nil {
		return StoredCheckpoint{}, ctx.Err()
	}
	if !validateCheckpoint(cp) || expected == ^uint64(0) {
		return StoredCheckpoint{}, ErrInvalidCheckpoint
	}
	sc, err := s.Load(ctx, cp.State.RunID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		return StoredCheckpoint{}, ctx.Err()
	}
	if s.records == nil {
		s.records = make(map[string]StoredCheckpoint)
	}
	if expected == 0 {
		if !errors.Is(err, ErrCheckpointNotFound) {
			return StoredCheckpoint{}, ErrCheckpointConflict
		}
		s.records[cp.State.RunID] = StoredCheckpoint{Version: 1, Checkpoint: cp.Clone()}
		return StoredCheckpoint{Version: 1, Checkpoint: cp.Clone()}, nil
	}
	if errors.Is(err, ErrCheckpointNotFound) || sc.Version != expected {
		return StoredCheckpoint{}, ErrCheckpointConflict
	}
	if err != nil {
		return StoredCheckpoint{}, err
	}
	s.records[cp.State.RunID] = StoredCheckpoint{Version: sc.Version + 1, Checkpoint: cp.Clone()}
	return StoredCheckpoint{Version: sc.Version + 1, Checkpoint: cp.Clone()}, nil
}

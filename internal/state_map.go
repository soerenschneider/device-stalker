package internal

import "sync"

type MapState struct {
	state map[string]bool
	mut   sync.RWMutex
}

func NewMapState() (*MapState, error) {
	return &MapState{
		state: map[string]bool{},
	}, nil
}

func (s *MapState) HasStateChanged(id string, newState bool) bool {
	s.mut.RLock()
	oldState, ok := s.state[id]
	s.mut.RUnlock()

	if !ok {
		// no entry found for id - make sure we detect an update
		oldState = !newState
	}

	if oldState == newState {
		return false
	}

	s.mut.Lock()
	defer s.mut.Unlock()
	s.state[id] = newState
	return true
}

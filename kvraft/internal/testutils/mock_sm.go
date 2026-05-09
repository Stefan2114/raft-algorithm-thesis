package testutils

import (
	"bytes"
	"encoding/gob"
	"sync"
)

type MockStateMachine struct {
	mu      sync.Mutex
	applied []any
}

func NewMockStateMachine() *MockStateMachine {
	return &MockStateMachine{
		applied: make([]any, 0),
	}
}

func (m *MockStateMachine) DoOp(req any) any {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.applied = append(m.applied, req)
	return req
}

func (m *MockStateMachine) Snapshot() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w := new(bytes.Buffer)
	enc := gob.NewEncoder(w)
	if err := enc.Encode(m.applied); err != nil {
		return nil, err
	}
	return w.Bytes(), nil
}

func (m *MockStateMachine) Restore(data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(data) == 0 {
		m.applied = make([]any, 0)
		return nil
	}
	dec := gob.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&m.applied); err != nil {
		return err
	}
	return nil
}

func (m *MockStateMachine) GetAppliedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.applied)
}

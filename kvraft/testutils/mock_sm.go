package testutils

import (
	"bytes"
	"encoding/gob"
	"sync"
)

type MockStateMachine struct {
	mu      sync.Mutex
	applied []interface{}
}

func NewMockStateMachine() *MockStateMachine {
	return &MockStateMachine{
		applied: make([]interface{}, 0),
	}
}

func (m *MockStateMachine) DoOp(req interface{}) interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.applied = append(m.applied, req)
	return req
}

func (m *MockStateMachine) Snapshot() []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	w := new(bytes.Buffer)
	enc := gob.NewEncoder(w)
	if err := enc.Encode(m.applied); err != nil {
		panic(err)
	}
	return w.Bytes()
}

func (m *MockStateMachine) Restore(data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(data) == 0 {
		m.applied = make([]interface{}, 0)
		return
	}
	dec := gob.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&m.applied); err != nil {
		panic(err)
	}
}

func (m *MockStateMachine) GetAppliedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.applied)
}

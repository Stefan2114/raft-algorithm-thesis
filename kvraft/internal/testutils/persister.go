package testutils

import (
	"sync"
)

type MockPersister struct {
	mu        sync.Mutex
	raftState []byte
	snapshot  []byte
}

func NewMockPersister() *MockPersister {
	return &MockPersister{}
}

func clone(orig []byte) []byte {
	x := make([]byte, len(orig))
	copy(x, orig)
	return x
}

func (p *MockPersister) ReadRaftState() ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return clone(p.raftState), nil
}

func (p *MockPersister) RaftStateSize() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.raftState)
}

func (p *MockPersister) ReadSnapshot() ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return clone(p.snapshot), nil
}

func (p *MockPersister) SnapshotSize() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.snapshot)
}

func (p *MockPersister) Save(raftState []byte, snapshot []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.raftState = clone(raftState)
	p.snapshot = clone(snapshot)
	return nil
}

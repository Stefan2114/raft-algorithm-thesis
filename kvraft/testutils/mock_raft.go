package testutils

import (
	"kvraft/raftapi"
	"sync"
)

// MockRaft simulates a Raft instance that immediately commits whatever is Started.
type MockRaft struct {
	mu            sync.Mutex
	me            int
	applyCh       chan raftapi.ApplyMsg
	currentTerm   int
	isLeader      bool
	currentLeader int
	nextIndex     int
	persister     raftapi.Persister
	msgQueue      chan raftapi.ApplyMsg
}

func NewMockRaft(me int, applyCh chan raftapi.ApplyMsg, persister raftapi.Persister) *MockRaft {
	m := &MockRaft{
		me:            me,
		applyCh:       applyCh,
		currentTerm:   1,
		isLeader:      true,
		currentLeader: me,
		nextIndex:     1,
		persister:     persister,
		msgQueue:      make(chan raftapi.ApplyMsg, 1000),
	}
	go m.processQueue()
	return m
}

func (m *MockRaft) processQueue() {
	for msg := range m.msgQueue {
		if m.applyCh != nil {
			m.applyCh <- msg
		}
	}
}

func (m *MockRaft) GetState() (int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.currentTerm, m.isLeader
}

func (m *MockRaft) GetLeader() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.currentLeader
}

func (m *MockRaft) Start(command interface{}) (int, int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isLeader {
		return -1, -1, false
	}

	index := m.nextIndex
	term := m.currentTerm
	m.nextIndex++

	// Queue the message for background delivery
	m.msgQueue <- raftapi.ApplyMsg{
		CommandValid: true,
		Command:      command,
		CommandIndex: index,
	}

	return index, term, true
}

func (m *MockRaft) PersistBytes() int {
	return m.persister.RaftStateSize()
}

func (m *MockRaft) Snapshot(index int, snapshot []byte) {
	m.persister.Save([]byte("mock_raft_state"), snapshot)
}

func (m *MockRaft) Kill() {
	// noop
}

func (m *MockRaft) SetLeader(isLeader bool, currentLeader int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.isLeader = isLeader
	m.currentLeader = currentLeader
	m.currentTerm++
}

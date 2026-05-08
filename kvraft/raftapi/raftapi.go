package raftapi

type Raft interface {
	// Start agreement on a new log entry, and return the log index
	// for that entry, the term, and whether the peer is the leader.
	Start(command interface{}) (int, int, bool)

	// Ask a Raft for its current term, and whether it thinks it is leader
	GetState() (int, bool)

	// GetLeader returns the id of the current leader, or -1 if no leader is known.
	GetLeader() int

	Snapshot(index int, snapshot []byte)
	PersistBytes() int
	Kill()
}

type Persister interface {
	ReadRaftState() ([]byte, error)
	RaftStateSize() int
	ReadSnapshot() ([]byte, error)
	SnapshotSize() int
	Save(raftState []byte, snapshot []byte) error
}

type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandIndex int

	SnapshotValid bool
	Snapshot      []byte
	SnapshotTerm  int
	SnapshotIndex int
}

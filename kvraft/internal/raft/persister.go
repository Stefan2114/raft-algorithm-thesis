package raft

type Persister interface {
	ReadRaftState() ([]byte, error)
	RaftStateSize() int
	ReadSnapshot() ([]byte, error)
	SnapshotSize() int
	Save(raftState []byte, snapshot []byte) error
}

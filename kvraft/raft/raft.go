package raft

import (
	"fmt"
)

type Raft interface {
	Start(command any) (int, int, bool)
	State() (int, bool)
	Leader() int
	Snapshot(index int, snapshot []byte)
	PersistBytes() int
	Kill()
}

type RaftRPC interface {
	RequestVote(args *RequestVoteArgs, reply *RequestVoteReply)
	AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply)
	InstallSnapshot(args *InstallSnapshotArgs, reply *InstallSnapshotReply)
	State() (int, bool)
}

type Persister interface {
	ReadRaftState() ([]byte, error)
	RaftStateSize() int
	ReadSnapshot() ([]byte, error)
	SnapshotSize() int
	Save(raftState []byte, snapshot []byte) error
}

type Transport interface {
	Call(method string, args any, reply any) bool
}

type ApplyMsg struct {
	CommandValid bool
	Command      any
	CommandIndex int

	SnapshotValid bool
	Snapshot      []byte
	SnapshotTerm  int
	SnapshotIndex int
}

type RequestVoteArgs struct {
	Term         int
	CandidateId  int
	LastLogTerm  int
	LastLogIndex int
}

type RequestVoteReply struct {
	Term        int
	VoteGranted bool
}

type AppendEntriesArgs struct {
	Term         int
	LeaderId     int
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []Entry
	LeaderCommit int
}
type AppendEntriesReply struct {
	Term          int
	Success       bool
	ConflictIndex int
	ConflictTerm  int
}

type InstallSnapshotArgs struct {
	Term              int
	LeaderId          int
	LastIncludedIndex int
	LastIncludedTerm  int
	Data              []byte
}

type InstallSnapshotReply struct {
	Term int
}

type Entry struct {
	Index   int
	Term    int
	Command any
}

func (e Entry) String() string {
	return fmt.Sprintf("{Idx:%d Trm:%d}", e.Index, e.Term)
}

package raft

import "fmt"

type NodeState int

const (
	StateFollower  NodeState = 0
	StateCandidate NodeState = 1
	StateLeader    NodeState = 2
)

func (s NodeState) String() string {
	switch s {
	case StateFollower:
		return "Follower"
	case StateCandidate:
		return "Candidate"
	case StateLeader:
		return "Leader"
	default:
		return "Unknown"
	}
}

type Entry struct {
	Index   int
	Term    int
	Command interface{}
}

func (e Entry) String() string {
	return fmt.Sprintf("{Idx:%d Trm:%d}", e.Index, e.Term)
}

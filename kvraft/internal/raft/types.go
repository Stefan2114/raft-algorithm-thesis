package raft

import "fmt"

type NodeState int

const (
	StateFollower  NodeState = 0
	StateCandidate NodeState = 1
	StateLeader    NodeState = 2
)

type Entry struct {
	Index   int
	Term    int
	Command interface{}
}

func (e Entry) String() string {
	return fmt.Sprintf("{Idx:%d Trm:%d}", e.Index, e.Term)
}

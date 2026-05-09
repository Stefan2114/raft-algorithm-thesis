package raft

import (
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"kvraft/internal/metrics"
	"kvraft/raft"
	"strconv"

	"go.uber.org/zap"
)

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

type Raft struct {
	mu        sync.RWMutex
	logger    *zap.Logger
	peers     []raft.Transport
	persister raft.Persister
	me        int
	dead      int32

	applyCh        chan raft.ApplyMsg
	applyCond      *sync.Cond
	replicatorCond []*sync.Cond

	state         NodeState
	currentTerm   int
	votedFor      int
	currentLeader int
	logs          []raft.Entry

	commitIndex int
	lastApplied int
	nextIndex   []int
	matchIndex  []int

	lastIncludedIndex int
	lastIncludedTerm  int

	electionTimer  *time.Timer
	heartBeatTimer *time.Timer

	electionTimeoutMin  time.Duration
	electionTimeoutRand time.Duration
	heartbeatTimeout    time.Duration
}

func Make(peers []raft.Transport, me int,
	persister raft.Persister, applyCh chan raft.ApplyMsg, logger *zap.Logger,
	electionMin, electionRand, hb time.Duration) raft.Raft {

	rf := &Raft{
		peers:               peers,
		persister:           persister,
		me:                  me,
		dead:                0,
		applyCh:             applyCh,
		replicatorCond:      make([]*sync.Cond, len(peers)),
		state:               StateFollower,
		currentTerm:         0,
		votedFor:            -1,
		currentLeader:       -1,
		logs:                make([]raft.Entry, 1),
		nextIndex:           make([]int, len(peers)),
		matchIndex:          make([]int, len(peers)),
		electionTimeoutMin:  electionMin,
		electionTimeoutRand: electionRand,
		heartbeatTimeout:    hb,
		logger:              logger.With(zap.Int("node", me)),
	}

	rf.heartBeatTimer = time.NewTimer(rf.stableHeartbeatTimeout())
	rf.electionTimer = time.NewTimer(rf.randomizedElectionTimeout())

	for i := range peers {
		rf.replicatorCond[i] = sync.NewCond(&rf.mu)
		if i != me {
			go rf.replicator(i)
		}
	}

	rf.readPersist(persister.ReadRaftState())
	rf.applyCond = sync.NewCond(&rf.mu)

	go rf.ticker()
	go rf.applier()
	return rf
}

func (rf *Raft) State() (int, bool) {
	rf.mu.RLock()
	defer rf.mu.RUnlock()
	return rf.currentTerm, rf.state == StateLeader
}

func (rf *Raft) Leader() int {
	rf.mu.RLock()
	defer rf.mu.RUnlock()
	return rf.currentLeader
}

// even if the Raft instance has been killed,
// this function should return gracefully.
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
func (rf *Raft) Start(command any) (int, int, bool) {

	rf.mu.Lock()
	defer rf.mu.Unlock()
	if rf.state != StateLeader {
		return -1, -1, false
	}
	newIndex := rf.getLen()
	newTerm := rf.currentTerm
	entry := raft.Entry{
		Index:   newIndex,
		Term:    newTerm,
		Command: command,
	}
	rf.logs = append(rf.logs, entry)
	rf.persist()
	rf.logger.Debug("received new command", zap.Int("index", newIndex), zap.Int("term", newTerm))
	rf.signalBroadcastReplication(false)
	return newIndex, newTerm, true
}

func (rf *Raft) PersistBytes() int {
	rf.mu.RLock()
	defer rf.mu.RUnlock()
	return rf.persister.RaftStateSize()
}

func (rf *Raft) applier() {
	defer close(rf.applyCh)

	for !rf.killed() {
		rf.mu.Lock()
		for rf.lastApplied >= rf.commitIndex {
			rf.applyCond.Wait()
			if rf.killed() {
				rf.mu.Unlock()
				return
			}
		}

		if rf.lastApplied < rf.lastIncludedIndex {
			snapshot, _ := rf.persister.ReadSnapshot()
			msg := raft.ApplyMsg{
				SnapshotValid: true,
				Snapshot:      snapshot,
				SnapshotTerm:  rf.lastIncludedTerm,
				SnapshotIndex: rf.lastIncludedIndex,
			}
			rf.lastApplied = rf.lastIncludedIndex
			metrics.LastApplied.WithLabelValues(strconv.Itoa(rf.me)).Set(float64(rf.lastApplied))
			rf.mu.Unlock()
			rf.applyCh <- msg
			continue
		}

		start := rf.lastApplied + 1
		limit := rf.commitIndex

		pStart := rf.getPhysicalIndex(start)
		pLimit := rf.getPhysicalIndex(limit)

		entries := make([]raft.Entry, pLimit-pStart+1)
		copy(entries, rf.logs[pStart:pLimit+1])

		rf.lastApplied = limit
		metrics.LastApplied.WithLabelValues(strconv.Itoa(rf.me)).Set(float64(rf.lastApplied))
		rf.mu.Unlock()
		for _, entry := range entries {
			rf.applyCh <- raft.ApplyMsg{
				CommandValid: true,
				Command:      entry.Command,
				CommandIndex: entry.Index,
			}
		}
	}
}

func (rf *Raft) handleHigherTerm(term int) bool {
	if term > rf.currentTerm {
		rf.currentTerm = term
		metrics.CurrentTerm.WithLabelValues(strconv.Itoa(rf.me)).Set(float64(rf.currentTerm))
		rf.votedFor = -1
		rf.state = StateFollower
		metrics.RaftState.WithLabelValues(strconv.Itoa(rf.me)).Set(0)
		rf.persist()
		rf.resetElectionTimer()
		return true
	}
	return false
}

func (rf *Raft) signalApplier() {
	rf.applyCond.Signal()
}

func (rf *Raft) getLastLog() raft.Entry {
	return rf.logs[len(rf.logs)-1]
}

func (rf *Raft) getLen() int {
	return rf.lastIncludedIndex + len(rf.logs)
}

func (rf *Raft) hasEntryAt(index int, term int) bool {
	if index < rf.lastIncludedIndex || index >= rf.getLen() {
		return false
	}
	return rf.getLog(index).Term == term
}

func (rf *Raft) getLog(index int) raft.Entry {
	return rf.logs[rf.getPhysicalIndex(index)]
}

func (rf *Raft) getPhysicalIndex(index int) int {
	return index - rf.lastIncludedIndex
}

func (rf *Raft) randomizedElectionTimeout() time.Duration {
	ms := rf.electionTimeoutMin.Milliseconds() + rand.Int63()%rf.electionTimeoutRand.Milliseconds()
	return time.Duration(ms) * time.Millisecond
}

func (rf *Raft) stableHeartbeatTimeout() time.Duration {
	return rf.heartbeatTimeout
}

func (rf *Raft) Kill() {
	atomic.StoreInt32(&rf.dead, 1)
	rf.signalApplier()
}

func (rf *Raft) killed() bool {
	z := atomic.LoadInt32(&rf.dead)
	return z == 1
}

package rsm

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"kvraft/api"
	"kvraft/internal/raft"
	"kvraft/raftapi"

	"go.uber.org/zap"
)

var useRaftStateMachine bool // to plug in another raft besided raft1

type Op struct {
	Me  int
	Id  string // Unique ID to match Submit with the applied result
	Req any
}

type result struct {
	id  string
	val any
}

type pendingEntry struct {
	id   string
	term int
	ch   chan result
}

// A server (i.e., ../transport.go) that wants to replicate itself calls
// MakeRSM and must implement the StateMachine interface.  This
// interface allows the rsm package to interact with the server for
// server-specific operations: the server must implement DoOp to
// execute an operation (e.g., a Get or Put request), and
// Snapshot/Restore to snapshot and restore the server's state.
type StateMachine interface {
	DoOp(any) any
	Snapshot() []byte
	Restore([]byte)
}

type RSM struct {
	mu           sync.Mutex
	me           int
	rf           raftapi.Raft
	applyCh      chan raftapi.ApplyMsg
	maxRaftState int // snapshot if log grows this big
	sm           StateMachine
	pending      map[int]*pendingEntry
	lastApplied  int
	logger       *zap.Logger
}

// servers[] contains the ports of the set of
// servers that will cooperate via Raft to
// form the fault-tolerant key/value service.
//
// me is the index of the current server in servers[].
//
// the k/v server should store snapshots through the underlying Raft
// implementation, which should call persister.SaveStateAndSnapshot() to
// atomically save the Raft state along with the snapshot.
// The RSM should snapshot when Raft's saved state exceeds maxRaftState bytes,
// in order to allow Raft to garbage-collect its log. if maxRaftState is -1,
// you don't need to snapshot.
//
// MakeRSM() must return quickly, so it should start goroutines for
// any long-running work.
func MakeRSM(servers []raft.Transport, me int, persister raft.Persister, maxRaftState int, sm StateMachine, logger *zap.Logger) *RSM {
	rsm := &RSM{
		me:           me,
		maxRaftState: maxRaftState,
		applyCh:      make(chan raftapi.ApplyMsg),
		sm:           sm,
		pending:      make(map[int]*pendingEntry),
		logger:       logger.With(zap.Int("node", me), zap.String("component", "rsm")),
	}
	if !useRaftStateMachine {
		rsm.rf = raft.Make(servers, me, persister, rsm.applyCh, logger)
	}
	if snapshot, _ := persister.ReadSnapshot(); len(snapshot) > 0 {
		rsm.sm.Restore(snapshot)
	}
	rsm.logger.Info("RSM started", zap.Int("maxRaftState", maxRaftState))

	go rsm.reader()
	return rsm
}

func (rsm *RSM) Raft() raftapi.Raft {
	return rsm.rf
}

// Submit a command to Raft, and wait for it to be committed.  It
// should return ErrWrongLeader if client should find new leader and
// try again.
func (rsm *RSM) Submit(req any) (api.Err, any) {

	id := randValue(8)
	op := Op{Me: rsm.me, Id: id, Req: req}
	ch := make(chan result)
	rsm.mu.Lock()

	index, term, isLeader := rsm.rf.Start(op)
	if !isLeader {
		rsm.mu.Unlock()
		return api.ErrWrongLeader, nil
	}

	rsm.logger.Debug("command submitted", zap.String("id", id), zap.Int("index", index), zap.Int("term", term))
	rsm.pending[index] = &pendingEntry{id: id, term: term, ch: ch}
	rsm.mu.Unlock()

	defer func() {
		rsm.mu.Lock()
		delete(rsm.pending, index)
		rsm.mu.Unlock()
	}()

	select {
	case res, ok := <-ch:
		if !ok {
			rsm.logger.Debug("submit failed: channel closed", zap.String("id", id), zap.Int("index", index))
			return api.ErrWrongLeader, nil
		}
		if res.id != id {
			rsm.logger.Debug("submit failed: leader changed", zap.String("id", id), zap.Int("index", index), zap.String("actualId", res.id))
			return api.ErrWrongLeader, nil
		}
		rsm.logger.Debug("submit success", zap.String("id", id), zap.Int("index", index))
		return api.OK, res.val
	case <-time.After(10 * time.Second):
		rsm.mu.Lock()
		pending := rsm.dumpPending()
		rsm.mu.Unlock()
		rsm.logger.Warn("submit timeout", zap.String("id", id), zap.Int("index", index), zap.String("pending", pending))
		return api.ErrWrongLeader, nil
	}
}

func (rsm *RSM) dumpPending() string {
	var s string
	for idx, e := range rsm.pending {
		s += fmt.Sprintf("idx=%d id=%s term=%d | ", idx, e.id, e.term)
	}
	return s
}

func (rsm *RSM) reader() {
	for msg := range rsm.applyCh {
		if msg.SnapshotValid {
			rsm.handleSnapshot(msg)
		} else if msg.CommandValid {
			rsm.handleCommand(msg)
		} else {
			rsm.logger.Error("reader: invalid command msg")
		}
	}
	rsm.cleanup()
}

func (rsm *RSM) handleSnapshot(msg raftapi.ApplyMsg) {
	rsm.logger.Debug("reader: snapshot", zap.Int("index", msg.SnapshotIndex))
	rsm.mu.Lock()
	defer rsm.mu.Unlock()

	if msg.SnapshotIndex <= rsm.lastApplied {
		panic("RSM got here ups")
		return
	}

	rsm.sm.Restore(msg.Snapshot)
	rsm.lastApplied = msg.SnapshotIndex
	rsm.notifyOutdated(msg.SnapshotIndex)
}

func (rsm *RSM) handleCommand(msg raftapi.ApplyMsg) {
	op, ok := msg.Command.(Op)

	if !ok {
		rsm.logger.Error("reader: command not Op type", zap.String("type", fmt.Sprintf("%T", msg.Command)))
		return
	}

	rsm.logger.Debug("reader: applying", zap.Int("index", msg.CommandIndex), zap.String("id", op.Id))

	rsm.mu.Lock()
	if msg.CommandIndex <= rsm.lastApplied {
		rsm.logger.Debug("reader: discarding stale index", zap.Int("index", msg.CommandIndex), zap.Int("lastApplied", rsm.lastApplied))
		rsm.mu.Unlock()
		return
	}
	rsm.lastApplied = msg.CommandIndex
	rsm.mu.Unlock()

	// Execute operation
	resultVal := rsm.sm.DoOp(op.Req)

	rsm.mu.Lock()
	rsm.notifyPending(msg.CommandIndex, op.Id, resultVal)
	rsm.checkSnapshot(msg.CommandIndex)
	rsm.mu.Unlock()
}

func (rsm *RSM) notifyPending(index int, id string, val any) {
	_, isLeader := rsm.rf.GetState()
	entry, exists := rsm.pending[index]

	if exists {
		rsm.logger.Debug("reader: notifying pending", zap.Int("index", index), zap.String("id", id), zap.Bool("matches", entry.id == id))
		// Only succeed if we are still leader and the ID matches [cite: 135-136]
		if isLeader && entry.id == id {
			entry.ch <- result{id: id, val: val}
		} else {
			entry.ch <- result{id: ""} // Forces client retry
		}
		delete(rsm.pending, index)
	}
	rsm.notifyOutdated(index) // Clean up any other entries that can't possibly succeed now
}

func (rsm *RSM) notifyOutdated(index int) {
	currentTerm, isLeader := rsm.rf.GetState()
	for idx, entry := range rsm.pending {
		// if idx <= index || (!isLeader && entry.term < currentTerm) {
		if idx <= index || !isLeader || entry.term != currentTerm {
			if !isLeader && entry.term == currentTerm {
				panic("Got here ups2")
			}
			entry.ch <- result{id: ""}
			delete(rsm.pending, idx)
		}
	}
}

func (rsm *RSM) checkSnapshot(index int) {
	if rsm.maxRaftState != -1 && rsm.rf.PersistBytes() >= rsm.maxRaftState {
		rsm.logger.Info("taking snapshot", zap.Int("index", index), zap.Int("persistBytes", rsm.rf.PersistBytes()), zap.Int("threshold", rsm.maxRaftState))
		snapshot := rsm.sm.Snapshot()
		rsm.rf.Snapshot(index, snapshot)
	}
}

func (rsm *RSM) cleanup() {
	rsm.logger.Info("reader: applyCh closed, waking all pending")
	// applyCh closed: wake up all waiting Submit() calls
	rsm.mu.Lock()
	defer rsm.mu.Unlock()
	for _, entry := range rsm.pending {
		close(entry.ch)
	}
	rsm.pending = make(map[int]*pendingEntry)
}

func randValue(n int) string {
	const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Int63()%int64(len(letterBytes))]
	}
	return string(b)
}

package rsm

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"kvraft/sm"
	"sync"
	"time"

	"kvraft/api"
	"kvraft/internal/metrics"
	raftimpl "kvraft/internal/raft"
	"kvraft/raft"
	"strconv"

	"go.uber.org/zap"
)

var useRaftStateMachine bool // to plug in another instance besided raft

type result struct {
	id  int64
	val any
}

type pendingEntry struct {
	id   int64
	term int
	ch   chan result
}

type RSM struct {
	mu            sync.Mutex
	me            int
	rf            raft.Raft
	applyCh       chan raft.ApplyMsg
	maxRaftState  int
	sm            sm.StateMachine
	pending       map[int]*pendingEntry
	lastApplied   int
	logger        *zap.Logger
	submitTimeout time.Duration
}

func MakeRSM(servers []raft.Transport, me int, persister raft.Persister, maxRaftState int, sm sm.StateMachine, logger *zap.Logger,
	electionMin, electionRand, hb, submitTimeout time.Duration) *RSM {
	rsm := &RSM{
		me:            me,
		maxRaftState:  maxRaftState,
		applyCh:       make(chan raft.ApplyMsg),
		sm:            sm,
		pending:       make(map[int]*pendingEntry),
		logger:        logger.With(zap.Int("node", me), zap.String("component", "sm")),
		submitTimeout: submitTimeout,
	}
	if !useRaftStateMachine {
		rsm.rf = raftimpl.Make(servers, me, persister, rsm.applyCh, logger, electionMin, electionRand, hb)
	}
	if snapshot, _ := persister.ReadSnapshot(); len(snapshot) > 0 {
		if err := rsm.sm.Restore(snapshot); err != nil {
			rsm.logger.Fatal("failed to restore snapshot", zap.Error(err))
		}
	}
	rsm.logger.Info("RSM started", zap.Int("maxRaftState", maxRaftState))

	go rsm.reader()
	return rsm
}

func (rsm *RSM) Raft() raft.Raft {
	return rsm.rf
}

func (rsm *RSM) Submit(req any) (api.Err, any) {

	id := randValue()
	op := sm.Op{Me: rsm.me, Id: id, Req: req}
	ch := make(chan result)
	rsm.mu.Lock()

	metrics.ClientRequestsTotal.WithLabelValues(strconv.Itoa(rsm.me), "submit").Inc()

	index, term, isLeader := rsm.rf.Start(op)
	if !isLeader {
		leader := rsm.rf.Leader()
		rsm.mu.Unlock()
		return api.ErrWrongLeader, leader
	}

	rsm.logger.Debug("command submitted", zap.Int64("id", id), zap.Int("index", index), zap.Int("term", term))
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
			rsm.logger.Debug("submit failed: channel closed", zap.Int64("id", id), zap.Int("index", index))
			return api.ErrWrongLeader, rsm.rf.Leader()
		}
		if res.id != id {
			rsm.logger.Debug("submit failed: leader changed", zap.Int64("id", id), zap.Int("index", index), zap.Int64("actualId", res.id))
			return api.ErrWrongLeader, rsm.rf.Leader()
		}
		rsm.logger.Debug("submit success", zap.Int64("id", id), zap.Int("index", index))
		return api.OK, res.val
	case <-time.After(rsm.submitTimeout):
		rsm.mu.Lock()
		pending := rsm.dumpPending()
		rsm.mu.Unlock()
		rsm.logger.Warn("submit timeout", zap.Int64("id", id), zap.Int("index", index), zap.String("pending", pending))
		return api.ErrWrongLeader, rsm.rf.Leader()
	}
}

func (rsm *RSM) dumpPending() string {
	var s string
	for idx, e := range rsm.pending {
		s += fmt.Sprintf("idx=%d id=%d term=%d | ", idx, e.id, e.term)
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

func (rsm *RSM) handleSnapshot(msg raft.ApplyMsg) {
	rsm.logger.Debug("reader: snapshot", zap.Int("index", msg.SnapshotIndex))
	rsm.mu.Lock()
	defer rsm.mu.Unlock()

	if err := rsm.sm.Restore(msg.Snapshot); err != nil {
		rsm.logger.Fatal("failed to restore snapshot", zap.Error(err))
	}
	rsm.lastApplied = msg.SnapshotIndex
	rsm.notifyOutdated(msg.SnapshotIndex)
}

func (rsm *RSM) handleCommand(msg raft.ApplyMsg) {
	op, ok := msg.Command.(sm.Op)

	if !ok {
		rsm.logger.Error("reader: command not Op type", zap.String("type", fmt.Sprintf("%T", msg.Command)))
		return
	}

	rsm.logger.Debug("reader: applying", zap.Int("index", msg.CommandIndex), zap.Int64("id", op.Id))

	rsm.mu.Lock()
	if msg.CommandIndex <= rsm.lastApplied {
		rsm.logger.Debug("reader: discarding stale index", zap.Int("index", msg.CommandIndex), zap.Int("lastApplied", rsm.lastApplied))
		rsm.mu.Unlock()
		return
	}
	rsm.lastApplied = msg.CommandIndex
	rsm.mu.Unlock()

	resultVal := rsm.sm.DoOp(op.Req)

	rsm.mu.Lock()
	rsm.notifyPending(msg.CommandIndex, op.Id, resultVal)
	rsm.checkSnapshot(msg.CommandIndex)
	rsm.mu.Unlock()
}

func (rsm *RSM) notifyPending(index int, id int64, val any) {
	_, isLeader := rsm.rf.State()
	entry, exists := rsm.pending[index]

	if exists {
		rsm.logger.Debug("reader: notifying pending", zap.Int("index", index), zap.Int64("id", id), zap.Bool("matches", entry.id == id))
		if isLeader && entry.id == id {
			entry.ch <- result{id: id, val: val}
		} else {
			entry.ch <- result{id: -1} // Forces client retry
		}
		delete(rsm.pending, index)
	}
	rsm.notifyOutdated(index)
}

func (rsm *RSM) notifyOutdated(index int) {
	currentTerm, isLeader := rsm.rf.State()
	for idx, entry := range rsm.pending {
		if idx <= index || !isLeader || entry.term != currentTerm {
			entry.ch <- result{id: -1}
			delete(rsm.pending, idx)
		}
	}
}

func (rsm *RSM) checkSnapshot(index int) {
	if rsm.maxRaftState != -1 && rsm.rf.PersistBytes() >= rsm.maxRaftState {
		rsm.logger.Info("taking snapshot", zap.Int("index", index), zap.Int("persistBytes", rsm.rf.PersistBytes()), zap.Int("threshold", rsm.maxRaftState))
		snapshot, err := rsm.sm.Snapshot()
		if err != nil {
			rsm.logger.Fatal("failed to take snapshot", zap.Error(err))
		}
		rsm.rf.Snapshot(index, snapshot)
	}
}

func (rsm *RSM) cleanup() {
	rsm.logger.Info("reader: applyCh closed, waking all pending")
	rsm.mu.Lock()
	defer rsm.mu.Unlock()
	for _, entry := range rsm.pending {
		close(entry.ch)
	}
	rsm.pending = make(map[int]*pendingEntry)
}

func randValue() int64 {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return int64(binary.BigEndian.Uint64(b[:]))
}

func (rsm *RSM) LastApplied() int {
	rsm.mu.Lock()
	defer rsm.mu.Unlock()
	return rsm.lastApplied
}

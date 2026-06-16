package raft

import (
	"bytes"
	"encoding/gob"
	"kvraft/internal/metrics"
	"kvraft/raft"
	"strconv"

	"go.uber.org/zap"
)

func (rf *Raft) persist() {
	if rf.killed() {
		return
	}
	snapshot, _ := rf.persister.ReadSnapshot()
	err := rf.persister.Save(rf.encodeState(), snapshot)
	if err != nil {
		rf.logger.Fatal("failed to persist", zap.Error(err))
	}
}
func (rf *Raft) encodeState() []byte {
	w := new(bytes.Buffer)
	e := gob.NewEncoder(w)
	if err := e.Encode(rf.currentTerm); err != nil {
		rf.logger.Fatal("failed to encode state", zap.Error(err))
	}
	if err := e.Encode(rf.votedFor); err != nil {
		rf.logger.Fatal("failed to encode state", zap.Error(err))
	}
	if err := e.Encode(rf.logs); err != nil {
		rf.logger.Fatal("failed to encode state", zap.Error(err))
	}
	if err := e.Encode(rf.lastIncludedIndex); err != nil {
		rf.logger.Fatal("failed to encode state", zap.Error(err))
	}
	if err := e.Encode(rf.lastIncludedTerm); err != nil {
		rf.logger.Fatal("failed to encode state", zap.Error(err))
	}
	return w.Bytes()
}

func (rf *Raft) readPersist(data []byte, err error) {
	if err != nil {
		rf.logger.Fatal("failed to read raft state from disk", zap.Error(err))
	}
	if len(data) < 1 {
		return
	}
	r := bytes.NewBuffer(data)
	d := gob.NewDecoder(r)
	var currentTerm, votedFor, lastIncludedIndex, lastIncludedTerm int
	var logs []raft.Entry

	if err := d.Decode(&currentTerm); err != nil {
		rf.logger.Fatal("failed to decode state", zap.Error(err))
	}
	if err := d.Decode(&votedFor); err != nil {
		rf.logger.Fatal("failed to decode state", zap.Error(err))
	}
	if err := d.Decode(&logs); err != nil {
		rf.logger.Fatal("failed to decode state", zap.Error(err))
	}
	if err := d.Decode(&lastIncludedIndex); err != nil {
		rf.logger.Fatal("failed to decode state", zap.Error(err))
	}
	if err := d.Decode(&lastIncludedTerm); err != nil {
		rf.logger.Fatal("failed to decode state", zap.Error(err))
	}
	rf.currentTerm = currentTerm
	rf.votedFor = votedFor
	rf.logs = logs
	rf.lastIncludedIndex = lastIncludedIndex
	rf.lastIncludedTerm = lastIncludedTerm

	if lastIncludedIndex > rf.commitIndex {
		rf.commitIndex = lastIncludedIndex
		metrics.CommitIndex.WithLabelValues(strconv.Itoa(rf.me)).Set(float64(rf.commitIndex))
	}
	if lastIncludedIndex > rf.lastApplied {
		rf.lastApplied = lastIncludedIndex
		metrics.LastApplied.WithLabelValues(strconv.Itoa(rf.me)).Set(float64(rf.lastApplied))
	}
}

func (rf *Raft) Snapshot(index int, snapshot []byte) {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	if index <= rf.lastIncludedIndex || index > rf.commitIndex {
		return
	}
	rf.logs = append([]raft.Entry{}, rf.logs[rf.getPhysicalIndex(index):]...)
	rf.lastIncludedIndex = index
	rf.lastIncludedTerm = rf.getFirstLog().Term
	err := rf.persister.Save(rf.encodeState(), snapshot)
	if err != nil {
		rf.logger.Fatal("failed to persist", zap.Error(err))
	}
}

func (rf *Raft) InstallSnapshot(args *raft.InstallSnapshotArgs, reply *raft.InstallSnapshotReply) {

	rf.mu.Lock()
	defer rf.mu.Unlock()

	reply.Term = rf.currentTerm
	if args.Term < rf.currentTerm {
		return
	}

	if args.Term > rf.currentTerm {
		rf.currentTerm, rf.votedFor = args.Term, -1
		metrics.CurrentTerm.WithLabelValues(strconv.Itoa(rf.me)).Set(float64(rf.currentTerm))
		rf.persist()
	}

	rf.currentLeader = args.LeaderId
	if rf.state != StateFollower {
		rf.state = StateFollower
		metrics.RaftState.WithLabelValues(strconv.Itoa(rf.me)).Set(0)
	}
	rf.resetElectionTimer()

	if args.LastIncludedIndex <= rf.lastIncludedIndex {
		return
	}

	rf.truncateLogWithSnapshot(args.LastIncludedIndex, args.LastIncludedTerm)

	rf.lastIncludedIndex = args.LastIncludedIndex
	rf.lastIncludedTerm = args.LastIncludedTerm

	if args.LastIncludedIndex > rf.commitIndex {
		rf.commitIndex = args.LastIncludedIndex
		metrics.CommitIndex.WithLabelValues(strconv.Itoa(rf.me)).Set(float64(rf.commitIndex))
	}

	if err := rf.persister.Save(rf.encodeState(), args.Data); err != nil {
		rf.logger.Fatal("failed to persist snapshot", zap.Error(err))
	}
	rf.signalApplier()
}

func (rf *Raft) getFirstLog() raft.Entry {
	return rf.logs[0]
}

func (rf *Raft) genInstallSnapshotArgs() *raft.InstallSnapshotArgs {
	snapshot, _ := rf.persister.ReadSnapshot()
	return &raft.InstallSnapshotArgs{
		Term:              rf.currentTerm,
		LeaderId:          rf.me,
		LastIncludedIndex: rf.lastIncludedIndex,
		LastIncludedTerm:  rf.lastIncludedTerm,
		Data:              snapshot,
	}
}

func (rf *Raft) truncateLogWithSnapshot(index int, term int) {

	if index < rf.getLen() && rf.getLog(index).Term == term {
		rf.logs = append([]raft.Entry{}, rf.logs[rf.getPhysicalIndex(index):]...)
		return
	}

	rf.logs = []raft.Entry{{Index: index, Term: term}}

}

package raft

import (
	"kvraft/internal/metrics"
	"kvraft/raft"
	"strconv"

	"go.uber.org/zap"
)

func (rf *Raft) replicator(peer int) {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	for !rf.killed() {
		for !rf.needsReplication(peer) {
			rf.replicatorCond[peer].Wait()
		}
		rf.mu.Unlock()
		rf.replicateToPeer(peer)
		rf.mu.Lock()
	}
}

func (rf *Raft) needsReplication(peer int) bool {
	return rf.state == StateLeader && rf.matchIndex[peer] < rf.getLastLog().Index
}

func (rf *Raft) replicateToPeer(peer int) {

	rf.mu.RLock()
	if rf.state != StateLeader {
		rf.mu.RUnlock()
		return
	}

	if rf.nextIndex[peer] <= rf.lastIncludedIndex {

		args := rf.genInstallSnapshotArgs()
		rf.mu.RUnlock()

		reply := &raft.InstallSnapshotReply{}
		if ok := rf.sendInstallSnapshot(peer, args, reply); ok {
			rf.handleInstallSnapshotReply(peer, args, reply)
		}
		return
	}

	prevLogIndex := rf.nextIndex[peer] - 1
	args := rf.genAppendEntriesArgs(prevLogIndex)
	rf.mu.RUnlock()

	reply := new(raft.AppendEntriesReply)
	if ok := rf.sendAppendEntries(peer, args, reply); ok {
		rf.handleAppendEntriesReply(peer, args, reply)
	}
}

func (rf *Raft) genAppendEntriesArgs(prevLogIndex int) *raft.AppendEntriesArgs {

	pPrev := rf.getPhysicalIndex(prevLogIndex)
	entries := make([]raft.Entry, len(rf.logs)-(pPrev+1))
	copy(entries, rf.logs[pPrev+1:])

	return &raft.AppendEntriesArgs{
		Term:         rf.currentTerm,
		LeaderId:     rf.me,
		PrevLogIndex: prevLogIndex,
		PrevLogTerm:  rf.logs[pPrev].Term,
		Entries:      entries,
		LeaderCommit: rf.commitIndex,
	}
}

func (rf *Raft) handleAppendEntriesReply(peer int, args *raft.AppendEntriesArgs, reply *raft.AppendEntriesReply) {

	rf.mu.Lock()
	defer rf.mu.Unlock()

	if rf.shouldIgnoreReply(args.Term, reply.Term) {
		return
	}

	if !reply.Success {
		rf.resolveConflict(peer, args, reply)
		return
	}

	rf.advancePeerIndices(peer, args)
	rf.updateCommitIndex()
}

func (rf *Raft) advanceCommitIndex(leaderCommit int) {
	if leaderCommit > rf.commitIndex {
		lastIndex := rf.getLen() - 1
		if leaderCommit < lastIndex {
			rf.commitIndex = leaderCommit
		} else {
			rf.commitIndex = lastIndex
		}
		metrics.CommitIndex.WithLabelValues(strconv.Itoa(rf.me)).Set(float64(rf.commitIndex))
		rf.signalApplier()
	}
}

func (rf *Raft) resolveConflict(peer int, args *raft.AppendEntriesArgs, reply *raft.AppendEntriesReply) {

	if args.PrevLogIndex < rf.lastIncludedIndex {
		rf.signalReplication(peer)
		return
	}

	if reply.ConflictTerm == -1 {
		rf.nextIndex[peer] = reply.ConflictIndex
	} else {
		// Search for the last index of the conflicting term in our own log
		lastIdx := rf.findLastIndexOfTerm(reply.ConflictTerm, args.PrevLogIndex)
		if lastIdx > 0 {
			rf.nextIndex[peer] = lastIdx + 1
		} else {
			rf.nextIndex[peer] = reply.ConflictIndex
		}
	}
	rf.logger.Debug("resolved replication conflict", zap.Int("peer", peer), zap.Int("nextIndex", rf.nextIndex[peer]))
	rf.signalReplication(peer)
}

func (rf *Raft) findLastIndexOfTerm(term int, startSearch int) int {
	for i := startSearch; i > rf.lastIncludedIndex; i-- {
		if rf.getLog(i).Term == term {
			return i
		}
	}
	return -1
}

func (rf *Raft) AppendEntries(args *raft.AppendEntriesArgs, reply *raft.AppendEntriesReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	defer func() {
		rf.logger.Debug("appendEntries finished",
			zap.String("state", rf.state.String()),
			zap.Int("term", rf.currentTerm),
			zap.Int("commitIndex", rf.commitIndex),
			zap.Bool("success", reply.Success))
	}()

	reply.Term, reply.Success = rf.currentTerm, false

	if args.Term < rf.currentTerm {
		return
	}
	defer rf.persist()

	if args.Term > rf.currentTerm {
		rf.currentTerm, rf.votedFor = args.Term, -1
		metrics.CurrentTerm.WithLabelValues(strconv.Itoa(rf.me)).Set(float64(rf.currentTerm))
	}

	rf.currentLeader = args.LeaderId
	if rf.state != StateFollower {
		rf.state = StateFollower
		metrics.RaftState.WithLabelValues(strconv.Itoa(rf.me)).Set(0)
	}
	rf.resetElectionTimer()

	if hasConflict := rf.handleConsistencyConflict(args, reply); hasConflict {
		return
	}
	rf.appendNewEntries(args.Entries)
	rf.advanceCommitIndex(args.LeaderCommit)
	reply.Success = true
}

func (rf *Raft) RequestVote(args *raft.RequestVoteArgs, reply *raft.RequestVoteReply) {

	rf.mu.Lock()
	defer rf.mu.Unlock()
	defer func() {
		rf.logger.Debug("requestVote finished",
			zap.String("state", rf.state.String()),
			zap.Int("term", rf.currentTerm),
			zap.Int("commitIndex", rf.commitIndex),
			zap.Bool("voteGranted", reply.VoteGranted))
	}()

	reply.Term, reply.VoteGranted = rf.currentTerm, false

	if args.Term < rf.currentTerm {
		return
	}
	defer rf.persist()

	if args.Term > rf.currentTerm {
		rf.state = StateFollower
		metrics.RaftState.WithLabelValues(strconv.Itoa(rf.me)).Set(0)
		rf.currentTerm, rf.votedFor = args.Term, -1
		metrics.CurrentTerm.WithLabelValues(strconv.Itoa(rf.me)).Set(float64(rf.currentTerm))
	}

	canVote := rf.votedFor == -1 || rf.votedFor == args.CandidateId
	logUpToDate := rf.isLogUpToDate(args.LastLogTerm, args.LastLogIndex)
	if canVote && logUpToDate {
		reply.VoteGranted = true
		rf.votedFor = args.CandidateId
		rf.resetElectionTimer()
	}
}

func (rf *Raft) handleInstallSnapshotReply(peer int, args *raft.InstallSnapshotArgs, reply *raft.InstallSnapshotReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if isHigher := rf.handleHigherTerm(reply.Term); isHigher {
		return
	}
	if rf.state == StateLeader && rf.currentTerm == args.Term {
		newMatch := args.LastIncludedIndex
		newNext := newMatch + 1
		if newNext > rf.nextIndex[peer] {
			rf.nextIndex[peer] = newNext
		}
		if newMatch > rf.matchIndex[peer] {
			rf.matchIndex[peer] = newMatch
		}
		rf.signalReplication(peer)
	}
}

func (rf *Raft) handleConsistencyConflict(args *raft.AppendEntriesArgs, reply *raft.AppendEntriesReply) bool {

	if args.PrevLogIndex < rf.lastIncludedIndex {
		reply.ConflictIndex = rf.lastIncludedIndex + 1
		reply.ConflictTerm = -1
		return true
	}

	if rf.hasEntryAt(args.PrevLogIndex, args.PrevLogTerm) {
		return false
	}

	if args.PrevLogIndex >= rf.getLen() {
		reply.ConflictIndex = rf.getLen()
		reply.ConflictTerm = -1
		return true
	}

	reply.ConflictTerm = rf.getLog(args.PrevLogIndex).Term
	index := args.PrevLogIndex
	for index > rf.lastIncludedIndex && rf.getLog(index).Term == reply.ConflictTerm {
		index--
	}
	reply.ConflictIndex = index + 1
	return true
}

func (rf *Raft) appendNewEntries(entries []raft.Entry) {
	for i, entry := range entries {

		if entry.Index < rf.getLen() {
			// Rule 3: If an existing entry conflicts with a new one (same index
			// but different terms), delete the existing entry and all that follow it
			pIdx := rf.getPhysicalIndex(entry.Index)
			if rf.logs[pIdx].Term != entry.Term {
				rf.logs = rf.logs[:pIdx]
				rf.logs = append(rf.logs, entries[i:]...)
				break
			}
		} else {
			rf.logs = append(rf.logs, entries[i:]...)
			break
		}
	}
}

func (rf *Raft) advancePeerIndices(peer int, args *raft.AppendEntriesArgs) {
	newMatch := args.PrevLogIndex + len(args.Entries)
	if newMatch > rf.matchIndex[peer] {
		rf.matchIndex[peer] = newMatch
		rf.nextIndex[peer] = newMatch + 1
		rf.signalReplication(peer)
	}
}

func (rf *Raft) updateCommitIndex() {
	for n := rf.getLen() - 1; n > rf.commitIndex; n-- {
		if rf.getLog(n).Term == rf.currentTerm && rf.countNodesWithLogAt(n) > len(rf.peers)/2 {
			rf.commitIndex = n
			metrics.CommitIndex.WithLabelValues(strconv.Itoa(rf.me)).Set(float64(rf.commitIndex))
			rf.logger.Debug("commitIndex advanced", zap.Int("index", rf.commitIndex))
			rf.signalApplier()
			break
		}
	}
}

func (rf *Raft) shouldIgnoreReply(argsTerm int, replyTerm int) bool {
	if isHigher := rf.handleHigherTerm(replyTerm); isHigher {
		return true
	}
	return rf.state != StateLeader || rf.currentTerm != argsTerm
}

func (rf *Raft) signalReplication(peer int) {
	rf.replicatorCond[peer].Signal()
}

func (rf *Raft) signalBroadcastReplication(isHeartbeat bool) {
	for peer := range rf.peers {
		if peer == rf.me {
			continue
		}
		if isHeartbeat {
			go rf.replicateToPeer(peer)
		} else {
			rf.signalReplication(peer)
		}
	}
}

func (rf *Raft) countNodesWithLogAt(index int) int {
	count := 1 // Count myself
	for i, mIndex := range rf.matchIndex {
		if i != rf.me && mIndex >= index {
			count++
		}
	}
	return count
}

func (rf *Raft) isLogUpToDate(candidateTerm int, candidateIndex int) bool {
	lastLog := rf.getLastLog()
	if candidateTerm != lastLog.Term {
		return candidateTerm > lastLog.Term
	}
	return candidateIndex >= lastLog.Index
}

func (rf *Raft) sendRequestVote(server int, args *raft.RequestVoteArgs, reply *raft.RequestVoteReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}

func (rf *Raft) sendAppendEntries(server int, args *raft.AppendEntriesArgs, reply *raft.AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	return ok
}

func (rf *Raft) sendInstallSnapshot(peer int, args *raft.InstallSnapshotArgs, reply *raft.InstallSnapshotReply) bool {
	ok := rf.peers[peer].Call("Raft.InstallSnapshot", args, reply)
	return ok
}

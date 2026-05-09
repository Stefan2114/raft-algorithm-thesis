package raft

import (
	"kvraft/internal/metrics"
	"kvraft/raft"
	"strconv"

	"go.uber.org/zap"
)

func (rf *Raft) ticker() {
	for rf.killed() == false {
		select {
		case <-rf.electionTimer.C:
			rf.mu.Lock()
			if rf.state == StateLeader {
				rf.resetElectionTimer()
			} else {
				rf.becomeCandidate()
			}
			rf.mu.Unlock()

		case <-rf.heartBeatTimer.C:
			rf.mu.Lock()
			if rf.state == StateLeader {
				rf.signalBroadcastReplication(true)
				rf.resetHeartbeatTimer()
			}
			rf.mu.Unlock()
		}
	}
}

func (rf *Raft) becomeCandidate() {

	rf.state = StateCandidate
	metrics.RaftState.WithLabelValues(strconv.Itoa(rf.me)).Set(1)
	rf.currentTerm += 1
	metrics.CurrentTerm.WithLabelValues(strconv.Itoa(rf.me)).Set(float64(rf.currentTerm))
	rf.persist()
	rf.startElection()
	rf.resetElectionTimer()
}

func (rf *Raft) becomeLeader() {

	rf.state = StateLeader
	metrics.RaftState.WithLabelValues(strconv.Itoa(rf.me)).Set(2)
	metrics.LeaderChangesTotal.WithLabelValues(strconv.Itoa(rf.me)).Inc()
	rf.currentLeader = rf.me
	lastIndex := rf.getLastLog().Index
	for i := range rf.peers {
		rf.nextIndex[i] = lastIndex + 1
		rf.matchIndex[i] = rf.lastIncludedIndex
	}
	rf.matchIndex[rf.me] = lastIndex
	rf.signalBroadcastReplication(true)
	rf.resetHeartbeatTimer()
}

func (rf *Raft) startElection() {

	rf.votedFor = rf.me
	rf.currentLeader = -1
	rf.persist()

	args := rf.genRequestVoteArgs()
	rf.logger.Info("starting election", zap.Int("term", rf.currentTerm))

	grantedVotes := 1

	for peer := range rf.peers {
		if peer == rf.me {
			continue
		}
		go rf.requestVoteFromPeer(peer, args, &grantedVotes)
	}
}

func (rf *Raft) requestVoteFromPeer(peer int, args *raft.RequestVoteArgs, grantedVotes *int) {
	reply := new(raft.RequestVoteReply)
	ok := rf.sendRequestVote(peer, args, reply)
	if !ok {
		return
	}

	rf.mu.Lock()
	defer rf.mu.Unlock()

	rf.logger.Debug("received RequestVoteResponse", zap.Int("peer", peer), zap.Bool("granted", reply.VoteGranted))

	if !rf.isStillValidCandidate(args.Term) {
		return
	}

	if isHigher := rf.handleHigherTerm(reply.Term); isHigher {
		rf.logger.Debug("found higher term, stepping down", zap.Int("term", reply.Term))
		return
	}

	if reply.VoteGranted {
		*grantedVotes++
		if *grantedVotes == (len(rf.peers)/2 + 1) {
			rf.logger.Info("achieved majority, becoming leader", zap.Int("term", rf.currentTerm))
			rf.becomeLeader()
		}
	}
}

func (rf *Raft) genRequestVoteArgs() *raft.RequestVoteArgs {
	lastLog := rf.getLastLog()
	return &raft.RequestVoteArgs{
		Term:         rf.currentTerm,
		CandidateId:  rf.me,
		LastLogIndex: lastLog.Index,
		LastLogTerm:  lastLog.Term,
	}
}

func (rf *Raft) isStillValidCandidate(electionTerm int) bool {
	return rf.state == StateCandidate && rf.currentTerm == electionTerm
}

func (rf *Raft) resetElectionTimer() {
	rf.electionTimer.Reset(rf.randomizedElectionTimeout())
}

func (rf *Raft) resetHeartbeatTimer() {
	rf.heartBeatTimer.Reset(rf.stableHeartbeatTimeout())
}

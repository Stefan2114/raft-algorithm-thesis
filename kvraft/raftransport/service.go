package raftransport

import (
	"context"

	"kvraft/internal/raft"
	kvpb "kvraft/pb"
)

// RaftService wraps *raft.Raft RPC handlers for gRPC.
type RaftService struct {
	kvpb.UnimplementedRaftServer
	RF *raft.Raft
}

func (s *RaftService) RequestVote(_ context.Context, req *kvpb.RequestVoteArgs) (*kvpb.RequestVoteReply, error) {
	args := raft.RequestVoteArgs{
		Term:         int(req.Term),
		CandidateId:  int(req.CandidateId),
		LastLogTerm:  int(req.LastLogTerm),
		LastLogIndex: int(req.LastLogIndex),
	}
	var reply raft.RequestVoteReply
	s.RF.RequestVote(&args, &reply)
	return &kvpb.RequestVoteReply{
		Term:        int32(reply.Term),
		VoteGranted: reply.VoteGranted,
	}, nil
}

func (s *RaftService) AppendEntries(_ context.Context, req *kvpb.AppendEntriesArgs) (*kvpb.AppendEntriesReply, error) {
	entries, err := entriesFromProto(req.Entries)
	if err != nil {
		term, _ := s.RF.GetState()
		return &kvpb.AppendEntriesReply{Term: int32(term), Success: false}, nil
	}
	args := raft.AppendEntriesArgs{
		Term:         int(req.Term),
		LeaderId:     int(req.LeaderId),
		PrevLogIndex: int(req.PrevLogIndex),
		PrevLogTerm:  int(req.PrevLogTerm),
		Entries:      entries,
		LeaderCommit: int(req.LeaderCommit),
	}
	var reply raft.AppendEntriesReply
	s.RF.AppendEntries(&args, &reply)
	return &kvpb.AppendEntriesReply{
		Term:          int32(reply.Term),
		Success:       reply.Success,
		ConflictIndex: int32(reply.ConflictIndex),
		ConflictTerm:  int32(reply.ConflictTerm),
	}, nil
}

func (s *RaftService) InstallSnapshot(_ context.Context, req *kvpb.InstallSnapshotArgs) (*kvpb.InstallSnapshotReply, error) {
	args := raft.InstallSnapshotArgs{
		Term:              int(req.Term),
		LeaderId:          int(req.LeaderId),
		LastIncludedIndex: int(req.LastIncludedIndex),
		LastIncludedTerm:  int(req.LastIncludedTerm),
		Data:              req.Data,
	}
	var reply raft.InstallSnapshotReply
	s.RF.InstallSnapshot(&args, &reply)
	return &kvpb.InstallSnapshotReply{Term: int32(reply.Term)}, nil
}

package raftransport

import (
	"context"
	"time"

	"kvraft/internal/raft"
	kvpb "kvraft/pb"
)

const rpcTimeout = 2 * time.Second

// GRPCClient implements raft.Transport using a Raft gRPC client stub.
type GRPCClient struct {
	Raft kvpb.RaftClient
}

func (c *GRPCClient) Call(method string, args interface{}, reply interface{}) bool {
	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
	defer cancel()

	switch method {
	case "Raft.RequestVote":
		a := args.(*raft.RequestVoteArgs)
		r := reply.(*raft.RequestVoteReply)
		res, err := c.Raft.RequestVote(ctx, &kvpb.RequestVoteArgs{
			Term:         int32(a.Term),
			CandidateId:  int32(a.CandidateId),
			LastLogTerm:  int32(a.LastLogTerm),
			LastLogIndex: int32(a.LastLogIndex),
		})
		if err != nil {
			return false
		}
		r.Term = int(res.Term)
		r.VoteGranted = res.VoteGranted
		return true

	case "Raft.AppendEntries":
		a := args.(*raft.AppendEntriesArgs)
		r := reply.(*raft.AppendEntriesReply)
		entries, err := entriesToProto(a.Entries)
		if err != nil {
			return false
		}
		res, err := c.Raft.AppendEntries(ctx, &kvpb.AppendEntriesArgs{
			Term:         int32(a.Term),
			LeaderId:     int32(a.LeaderId),
			PrevLogIndex: int32(a.PrevLogIndex),
			PrevLogTerm:  int32(a.PrevLogTerm),
			Entries:      entries,
			LeaderCommit: int32(a.LeaderCommit),
		})
		if err != nil {
			return false
		}
		r.Term = int(res.Term)
		r.Success = res.Success
		r.ConflictIndex = int(res.ConflictIndex)
		r.ConflictTerm = int(res.ConflictTerm)
		return true

	case "Raft.InstallSnapshot":
		a := args.(*raft.InstallSnapshotArgs)
		r := reply.(*raft.InstallSnapshotReply)
		res, err := c.Raft.InstallSnapshot(ctx, &kvpb.InstallSnapshotArgs{
			Term:              int32(a.Term),
			LeaderId:          int32(a.LeaderId),
			LastIncludedIndex: int32(a.LastIncludedIndex),
			LastIncludedTerm:  int32(a.LastIncludedTerm),
			Data:              a.Data,
		})
		if err != nil {
			return false
		}
		r.Term = int(res.Term)
		return true

	default:
		return false
	}
}

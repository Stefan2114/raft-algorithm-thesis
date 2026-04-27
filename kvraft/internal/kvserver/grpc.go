package kvserver

import (
	"context"

	"kvraft/api"
	"kvraft/internal/rsm"
	kvpb "kvraft/pb"
)

func errToStatus(e api.Err) kvpb.Status {
	switch e {
	case api.OK:
		return kvpb.Status_OK
	case api.ErrNoKey:
		return kvpb.Status_ERR_NO_KEY
	case api.ErrVersion:
		return kvpb.Status_ERR_VERSION
	case api.ErrWrongLeader:
		return kvpb.Status_ERR_WRONG_LEADER
	case api.ErrMaybe:
		return kvpb.Status_ERR_MAYBE
	default:
		return kvpb.Status_STATUS_UNSPECIFIED
	}
}

// KVService serves client Get/Put over gRPC by submitting through the RSM.
type KVService struct {
	kvpb.UnimplementedKVServer
	RSM *rsm.RSM
}

func (s *KVService) Get(ctx context.Context, req *kvpb.GetRequest) (*kvpb.GetResponse, error) {
	_ = ctx
	code, val := s.RSM.Submit(api.GetArgs{Key: req.Key})
	if code == api.ErrWrongLeader {
		var leaderHint int32 = -1
		if leader, ok := val.(int); ok {
			leaderHint = int32(leader)
		}
		return &kvpb.GetResponse{Status: errToStatus(code), LeaderHint: leaderHint}, nil
	}
	gr, ok := val.(api.GetReply)
	if !ok {
		return &kvpb.GetResponse{Status: kvpb.Status_ERR_WRONG_LEADER}, nil
	}
	return &kvpb.GetResponse{
		Status:  errToStatus(gr.Err),
		Value:   gr.Value,
		Version: uint64(gr.Version),
	}, nil
}

func (s *KVService) Put(ctx context.Context, req *kvpb.PutRequest) (*kvpb.PutResponse, error) {
	_ = ctx
	code, val := s.RSM.Submit(api.PutArgs{
		Key:     req.Key,
		Value:   req.Value,
		Version: api.TVersion(req.Version),
	})
	if code == api.ErrWrongLeader {
		var leaderHint int32 = -1
		if leader, ok := val.(int); ok {
			leaderHint = int32(leader)
		}
		return &kvpb.PutResponse{Status: errToStatus(code), LeaderHint: leaderHint}, nil
	}
	pr, ok := val.(api.PutReply)
	if !ok {
		return &kvpb.PutResponse{Status: kvpb.Status_ERR_WRONG_LEADER}, nil
	}
	return &kvpb.PutResponse{Status: errToStatus(pr.Err)}, nil
}

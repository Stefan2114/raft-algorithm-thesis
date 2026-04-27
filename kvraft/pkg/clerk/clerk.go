package kvclient

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"kvraft/api"
	kvpb "kvraft/pb"
)

const rpcTimeout = 5 * time.Second

type Clerk struct {
	addresses   []string
	lastLeader  int
	connections []*grpc.ClientConn
	clients     []kvpb.KVClient
}

func NewClerk(addresses []string) (*Clerk, error) {
	connections := make([]*grpc.ClientConn, len(addresses))
	clients := make([]kvpb.KVClient, len(addresses))
	for i, a := range addresses {
		conn, err := grpc.NewClient(a, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			for _, c := range connections {
				if c != nil {
					_ = c.Close()
				}
			}
			return nil, err
		}
		connections[i] = conn
		clients[i] = kvpb.NewKVClient(conn)
	}
	return &Clerk{
		addresses:   addresses,
		lastLeader:  0,
		connections: connections,
		clients:     clients,
	}, nil
}

func (ck *Clerk) Close() {
	for _, c := range ck.connections {
		if c != nil {
			_ = c.Close()
		}
	}
}

func pbStatusToErr(s kvpb.Status) api.Err {
	switch s {
	case kvpb.Status_OK:
		return api.OK
	case kvpb.Status_ERR_NO_KEY:
		return api.ErrNoKey
	case kvpb.Status_ERR_VERSION:
		return api.ErrVersion
	case kvpb.Status_ERR_WRONG_LEADER:
		return api.ErrWrongLeader
	case kvpb.Status_ERR_MAYBE:
		return api.ErrMaybe
	default:
		return api.ErrWrongLeader
	}
}

func (ck *Clerk) Get(key string) (string, api.TVersion, api.Err) {
	ctx := context.Background()
	srv := ck.lastLeader
	tried := 0
	for {
		cctx, cancel := context.WithTimeout(ctx, rpcTimeout)
		resp, err := ck.clients[srv].Get(cctx, &kvpb.GetRequest{Key: key})
		cancel()
		if err == nil && resp.Status != kvpb.Status_ERR_WRONG_LEADER {
			ck.lastLeader = srv
			return resp.Value, api.TVersion(resp.Version), pbStatusToErr(resp.Status)
		}
		tried++
		if err == nil && resp != nil && resp.LeaderHint >= 0 && int(resp.LeaderHint) < len(ck.clients) && int(resp.LeaderHint) != srv {
			srv = int(resp.LeaderHint)
		} else {
			srv = (srv + 1) % len(ck.clients)
		}
		
		if tried >= len(ck.clients) {
			time.Sleep(20 * time.Millisecond)
			tried = 0
		}
	}
}

func (ck *Clerk) Put(key string, value string, version api.TVersion) api.Err {
	ctx := context.Background()
	srv := ck.lastLeader
	firstAttempt := true
	tried := 0
	for {
		cctx, cancel := context.WithTimeout(ctx, rpcTimeout)
		resp, err := ck.clients[srv].Put(cctx, &kvpb.PutRequest{
			Key:     key,
			Value:   value,
			Version: uint64(version),
		})
		cancel()
		if err == nil {
			switch pbStatusToErr(resp.Status) {
			case api.OK:
				ck.lastLeader = srv
				return api.OK
			case api.ErrVersion:
				ck.lastLeader = srv
				if firstAttempt {
					return api.ErrVersion
				}
				return api.ErrMaybe
			case api.ErrWrongLeader:
				firstAttempt = false
			default:
				firstAttempt = false
			}
		} else {
			firstAttempt = false
		}
		tried++
		if err == nil && resp != nil && resp.LeaderHint >= 0 && int(resp.LeaderHint) < len(ck.clients) && int(resp.LeaderHint) != srv {
			srv = int(resp.LeaderHint)
		} else {
			srv = (srv + 1) % len(ck.clients)
		}
		
		if tried >= len(ck.clients) {
			time.Sleep(20 * time.Millisecond)
			tried = 0
		}
	}
}

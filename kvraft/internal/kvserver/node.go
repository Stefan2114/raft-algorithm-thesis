package kvserver

import (
	"fmt"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"

	"kvraft/config"
	"kvraft/internal/logger"
	"kvraft/internal/raft"
	"kvraft/internal/rsm"
	kvpb "kvraft/pb"
	"kvraft/persist"
	"kvraft/raftransport"
)

// Node runs one replicated KV replica (Raft + RSM + gRPC).
type Node struct {
	me          int
	lis         net.Listener
	grpcSrv     *grpc.Server
	rsm         *rsm.RSM
	raftCore    *raft.Raft
	connections []*grpc.ClientConn
}

func dialPeer(addr string) (*grpc.ClientConn, error) {
	return grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff: backoff.Config{
				BaseDelay:  100 * time.Millisecond,
				Multiplier: 1.6,
				Jitter:     0.2,
				MaxDelay:   3 * time.Second,
			},
			MinConnectTimeout: 2 * time.Second,
		}),
	)
}

// StartNode listens on the address of the node with the given config id and joins the cluster.
func StartNode(cfg *config.Config, nodeID int, dataDir string, maxRaftState int, isProd bool, isDebug bool, logPath string) (*Node, error) {
	me := -1
	for i, n := range cfg.Nodes {
		if n.ID == nodeID {
			me = i
			break
		}
	}
	if me < 0 {
		return nil, fmt.Errorf("unknown node id %d", nodeID)
	}
	raftransport.RegisterRaftGobTypes()

	ps, err := persist.MakeFilePersister(dataDir)
	if err != nil {
		return nil, err
	}

	transports := make([]raft.Transport, len(cfg.Nodes))
	connections := make([]*grpc.ClientConn, 0, len(cfg.Nodes)-1)
	for i := range cfg.Nodes {
		if i == me {
			transports[i] = raftransport.Noop{}
			continue
		}
		conn, err := dialPeer(cfg.Nodes[i].Addr)
		if err != nil {
			for _, c := range connections {
				_ = c.Close()
			}
			return nil, fmt.Errorf("peer %d: %w", i, err)
		}
		connections = append(connections, conn)
		transports[i] = &raftransport.GRPCClient{Raft: kvpb.NewRaftClient(conn)}
	}

	raftLogger := logger.InitLogger(isProd, isDebug, logPath)

	store := NewStore()
	rsmInst := rsm.MakeRSM(transports, me, ps, maxRaftState, store, raftLogger)

	rf, ok := rsmInst.Raft().(*raft.Raft)
	if !ok {
		return nil, fmt.Errorf("raft concrete type assertion failed")
	}

	lis, err := net.Listen("tcp", cfg.Nodes[me].Addr)
	if err != nil {
		for _, c := range connections {
			_ = c.Close()
		}
		return nil, err
	}

	srv := grpc.NewServer()
	kvpb.RegisterRaftServer(srv, &raftransport.RaftService{RF: rf})
	kvpb.RegisterKVServer(srv, &KVService{RSM: rsmInst})

	go func() {
		_ = srv.Serve(lis)
	}()

	return &Node{
		me:          me,
		lis:         lis,
		grpcSrv:     srv,
		rsm:         rsmInst,
		raftCore:    rf,
		connections: connections,
	}, nil
}

func (n *Node) Stop() {
	n.grpcSrv.GracefulStop()
	_ = n.lis.Close()
	for _, c := range n.connections {
		_ = c.Close()
	}
	if n.raftCore != nil {
		n.raftCore.Kill()
	}
}

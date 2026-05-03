package kvserver

import (
	"fmt"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"

	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"kvraft/config"
	"kvraft/internal/logger"
	"kvraft/internal/metrics"
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

func dialPeer(addr string, baseDelay, maxDelay, minConnectTimeout time.Duration) (*grpc.ClientConn, error) {
	return grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff: backoff.Config{
				BaseDelay:  baseDelay,
				Multiplier: 1.6,
				Jitter:     0.2,
				MaxDelay:   maxDelay,
			},
			MinConnectTimeout: minConnectTimeout,
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
		conn, err := dialPeer(cfg.Nodes[i].Addr,
			time.Duration(cfg.BaseDelay)*time.Millisecond,
			time.Duration(cfg.MaxDelay)*time.Second,
			time.Duration(cfg.MinConnectTimeout)*time.Second)
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
	rsmInst := rsm.MakeRSM(transports, me, ps, maxRaftState, store, raftLogger,
		time.Duration(cfg.ElectionTimeoutMin)*time.Millisecond,
		time.Duration(cfg.ElectionTimeoutRand)*time.Millisecond,
		time.Duration(cfg.HeartbeatTimeout)*time.Millisecond,
		time.Duration(cfg.SubmitTimeout)*time.Second)

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

	metrics.InitMetrics(me)
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	metricsServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.MetricsPortBase+nodeID),
		Handler: mux,
	}
	go func() {
		_ = metricsServer.ListenAndServe()
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

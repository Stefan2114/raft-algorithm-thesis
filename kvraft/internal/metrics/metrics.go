package metrics

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Raft State: 0=Follower, 1=Candidate, 2=Leader
	RaftState = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "raft_state",
		Help: "Current state of the Raft node (0=Follower, 1=Candidate, 2=Leader)",
	}, []string{"node_id"})

	CurrentTerm = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "raft_current_term",
		Help: "Current term of the Raft node",
	}, []string{"node_id"})

	CommitIndex = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "raft_commit_index",
		Help: "Current commit index of the Raft node",
	}, []string{"node_id"})

	LastApplied = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "raft_last_applied",
		Help: "Last applied index to the state machine",
	}, []string{"node_id"})

	ClientRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "raft_client_requests_total",
		Help: "Total number of client requests handled",
	}, []string{"node_id", "operation"})

	LeaderChangesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "raft_leader_changes_total",
		Help: "Total number of times a node became leader",
	}, []string{"node_id"})
)

// InitMetrics sets initial values for the metrics so they appear in Prometheus immediately.
func InitMetrics(nodeID int) {
	idStr := strconv.Itoa(nodeID)
	RaftState.WithLabelValues(idStr).Set(0) // Default to Follower
	CurrentTerm.WithLabelValues(idStr).Set(0)
	CommitIndex.WithLabelValues(idStr).Set(0)
	LastApplied.WithLabelValues(idStr).Set(0)
}

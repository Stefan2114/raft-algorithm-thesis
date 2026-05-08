package kvserver

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"kvraft/api"
	"kvraft/config"
	"kvraft/pkg/clerk"

	"github.com/anishathalye/porcupine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Porcupine model for our KV store
type kvInput struct {
	op      string // "get", "put"
	key     string
	val     string
	version api.TVersion
}

type kvOutput struct {
	val     string
	version api.TVersion
	err     api.Err
}

var kvModel = porcupine.Model{
	Init: func() interface{} {
		return make(map[string]struct {
			val     string
			version api.TVersion
		})
	},
	Step: func(state interface{}, input interface{}, output interface{}) (bool, interface{}) {
		st := state.(map[string]struct {
			val     string
			version api.TVersion
		})
		inp := input.(kvInput)
		out := output.(kvOutput)

		newSt := make(map[string]struct {
			val     string
			version api.TVersion
		})
		for k, v := range st {
			newSt[k] = v
		}

		if inp.op == "get" {
			cur, ok := st[inp.key]
			if !ok {
				if out.err == api.ErrNoKey {
					return true, newSt
				}
				return false, newSt
			}
			if out.err == api.OK && out.val == cur.val && out.version == cur.version {
				return true, newSt
			}
			return false, newSt
		} else if inp.op == "put" {
			cur, ok := st[inp.key]
			if !ok {
				if inp.version == 0 {
					if out.err == api.OK {
						newSt[inp.key] = struct {
							val     string
							version api.TVersion
						}{val: inp.val, version: 1}
						return true, newSt
					}
				} else {
					if out.err == api.ErrVersion {
						return true, newSt
					}
				}
				return false, newSt
			} else {
				if cur.version == inp.version {
					if out.err == api.OK {
						newSt[inp.key] = struct {
							val     string
							version api.TVersion
						}{val: inp.val, version: cur.version + 1}
						return true, newSt
					}
				} else {
					if out.err == api.ErrVersion {
						return true, newSt
					}
				}
				return false, newSt
			}
		}
		return false, newSt
	},
}

func TestLinearizability(t *testing.T) {
	nNodes := 3
	cfg := &config.Config{
		Nodes: make([]config.Node, nNodes),
	}
	for i := 0; i < nNodes; i++ {
		cfg.Nodes[i] = config.Node{
			ID:   i,
			Addr: fmt.Sprintf("localhost:%d", 10000+i),
		}
	}
	cfg.PopulateDefaults()

	tempDir, err := os.MkdirTemp("", "kvraft-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	nodes := make([]*Node, nNodes)
	for i := 0; i < nNodes; i++ {
		nodeDir := filepath.Join(tempDir, fmt.Sprintf("node-%d", i))
		node, err := StartNode(cfg, i, nodeDir, -1, false, true, "")
		require.NoError(t, err, "failed to start node %d", i)
		nodes[i] = node
	}
	defer func() {
		for _, n := range nodes {
			n.Stop()
		}
	}()

	// Wait for leader election
	time.Sleep(2 * time.Second)

	ck, err := kvclient.NewClerk([]string{
		cfg.Nodes[0].Addr,
		cfg.Nodes[1].Addr,
		cfg.Nodes[2].Addr,
	})
	require.NoError(t, err)
	defer ck.Close()

	nClients := 5
	nOpsPerClient := 20
	var wg sync.WaitGroup
	wg.Add(nClients)

	history := make([]porcupine.Operation, nClients*nOpsPerClient)
	historyMu := sync.Mutex{}
	opIdx := 0

	for c := 0; c < nClients; c++ {
		go func(clientId int) {
			defer wg.Done()
			for i := 0; i < nOpsPerClient; i++ {
				key := "key"
				opType := "put"
				if rand.Intn(2) == 0 {
					opType = "get"
				}

				var start, end time.Time
				var inp kvInput
				var out kvOutput

				if opType == "get" {
					inp = kvInput{op: "get", key: key}
					start = time.Now()
					val, ver, err := ck.Get(key)
					end = time.Now()
					out = kvOutput{val: val, version: ver, err: err}
				} else {
					_, ver, err := ck.Get(key)
					if err != api.OK && err != api.ErrNoKey {
						continue
					}
					if err == api.ErrNoKey {
						ver = 0
					}

					val := fmt.Sprintf("val-%d-%d", clientId, i)
					inp = kvInput{op: "put", key: key, val: val, version: ver}
					start = time.Now()
					err = ck.Put(key, val, ver)
					end = time.Now()
					out = kvOutput{err: err}
				}

				historyMu.Lock()
				history[opIdx] = porcupine.Operation{
					ClientId: clientId,
					Input:    inp,
					Call:     start.UnixNano(),
					Output:   out,
					Return:   end.UnixNano(),
				}
				opIdx++
				historyMu.Unlock()

				time.Sleep(time.Duration(rand.Intn(50)) * time.Millisecond)
			}
		}(c)
	}

	wg.Wait()

	history = history[:opIdx]

	res, info := porcupine.CheckOperationsVerbose(kvModel, history, 0)
	if res == porcupine.Illegal {
		visualPath := "linearizability-failure.html"
		err := porcupine.VisualizePath(kvModel, info, visualPath)
		assert.NoError(t, err, "Failed to generate visualization")
		assert.Fail(t, "History is NOT linearizable. Failure visualization saved to %s", visualPath)
	} else if res == porcupine.Unknown {
		t.Log("Linearizability check timed out or is unknown")
	} else {
		t.Log("Linearizability check passed")
	}
}
func TestSnapshotLaggingServer(t *testing.T) {
	nNodes := 3
	cfg := &config.Config{
		Nodes: make([]config.Node, nNodes),
	}
	for i := 0; i < nNodes; i++ {
		cfg.Nodes[i] = config.Node{
			ID:   i,
			Addr: fmt.Sprintf("localhost:%d", 12000+i),
		}
	}
	cfg.PopulateDefaults()

	tempDir, err := os.MkdirTemp("", "kvraft-snap-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	maxRaftState := 1000

	nodes := make([]*Node, nNodes)
	nodeDirs := make([]string, nNodes)
	for i := 0; i < nNodes; i++ {
		nodeDirs[i] = filepath.Join(tempDir, fmt.Sprintf("node-%d", i))
		node, err := StartNode(cfg, i, nodeDirs[i], maxRaftState, false, true, "")
		require.NoError(t, err, "failed to start node %d", i)
		nodes[i] = node
	}
	defer func() {
		for _, n := range nodes {
			if n != nil {
				n.Stop()
			}
		}
	}()

	time.Sleep(2 * time.Second)

	ck, _ := kvclient.NewClerk([]string{cfg.Nodes[0].Addr, cfg.Nodes[1].Addr, cfg.Nodes[2].Addr})
	defer ck.Close()

	ck.Put("k1", "v1", 0)

	nodes[2].Stop()
	nodes[2] = nil

	for i := 0; i < 100; i++ {
		ck.Put("k1", fmt.Sprintf("v%d", i), api.TVersion(i+1))
	}

	node2, err := StartNode(cfg, 2, nodeDirs[2], maxRaftState, false, true, "")
	require.NoError(t, err, "failed to restart node 2")
	nodes[2] = node2

	time.Sleep(2 * time.Second)

	val, ver, errCode := ck.Get("k1")
	assert.Equal(t, api.OK, errCode)
	assert.Equal(t, "v99", val)
	assert.Equal(t, api.TVersion(101), ver)
}

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
	"kvraft/internal/testutils"
	kvclient "kvraft/pkg/clerk"

	"github.com/anishathalye/porcupine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestCluster(t *testing.T, nNodes int, basePort int, maxRaftState int) (*config.Config, []*Node, string) {
	cfg := &config.Config{
		Nodes: make([]config.Node, nNodes),
	}
	for i := 0; i < nNodes; i++ {
		cfg.Nodes[i] = config.Node{
			ID:   i,
			Addr: fmt.Sprintf("localhost:%d", basePort+i),
		}
	}
	cfg.PopulateDefaults()

	tempDir, err := os.MkdirTemp("", "kvraft-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	nodes := make([]*Node, nNodes)
	for i := 0; i < nNodes; i++ {
		nodeDir := filepath.Join(tempDir, fmt.Sprintf("node-%d", i))
		node, err := StartNode(cfg, i, nodeDir, maxRaftState, false, true, "")
		require.NoError(t, err, "failed to start node %d", i)
		nodes[i] = node
	}

	t.Cleanup(func() {
		for _, n := range nodes {
			if n != nil {
				n.Stop()
			}
		}
	})

	// Wait for leader election
	time.Sleep(2 * time.Second)
	return cfg, nodes, tempDir
}

func verifyLinearizability(t *testing.T, history []porcupine.Operation, visualPath string) {
	res, info := porcupine.CheckOperationsVerbose(testutils.KvModel, history, 0)
	switch res {
	case porcupine.Illegal:
		err := porcupine.VisualizePath(testutils.KvModel, info, visualPath)
		assert.NoError(t, err, "Failed to generate visualization")
		assert.Fail(t, "History is NOT linearizable. Failure visualization saved to %s", visualPath)
	case porcupine.Unknown:
		t.Log("Linearizability check timed out or is unknown")
	default:
		t.Log("Linearizability check passed")
	}
}

func performRandomOp(ck *kvclient.Clerk, clientId int, opIdx int, key string) (testutils.KvInput, testutils.KvOutput, time.Time, time.Time) {
	opType := "put"
	if rand.Intn(2) == 0 {
		opType = "get"
	}

	var start, end time.Time
	var inp testutils.KvInput
	var out testutils.KvOutput

	if opType == "get" {
		inp = testutils.KvInput{Op: 0, Key: key}
		start = time.Now()
		val, ver, err := ck.Get(key)
		end = time.Now()
		out = testutils.KvOutput{Value: val, Version: uint64(ver), Err: string(err)}
	} else {
		_, ver, err := ck.Get(key)
		if err != api.OK && err != api.ErrNoKey {
			return inp, out, time.Time{}, time.Time{}
		}
		if err == api.ErrNoKey {
			ver = 0
		}

		val := fmt.Sprintf("val-%d-%d", clientId, opIdx)
		inp = testutils.KvInput{Op: 1, Key: key, Value: val, Version: uint64(ver)}
		start = time.Now()
		err = ck.Put(key, val, ver)
		end = time.Now()
		out = testutils.KvOutput{Err: string(err)}
	}
	return inp, out, start, end
}

func runClientWorker(clientId int, ck *kvclient.Clerk, nOps int, key string, sleepMs int, history []porcupine.Operation, historyMu *sync.Mutex, opIdxPtr *int, wg *sync.WaitGroup) {
	defer wg.Done()
	for i := 0; i < nOps; i++ {
		inp, out, start, end := performRandomOp(ck, clientId, i, key)
		if start.IsZero() {
			continue
		}

		historyMu.Lock()
		if *opIdxPtr < len(history) {
			history[*opIdxPtr] = porcupine.Operation{
				ClientId: clientId,
				Input:    inp,
				Call:     start.UnixNano(),
				Output:   out,
				Return:   end.UnixNano(),
			}
			*opIdxPtr++
		}
		historyMu.Unlock()
		time.Sleep(time.Duration(rand.Intn(sleepMs)) * time.Millisecond)
	}
}

func startChaos(stopCh chan struct{}, nNodes int, nodes []*Node, cfg *config.Config, tempDir string) {
	mu := sync.Mutex{}
	go func() {
		for {
			select {
			case <-stopCh:
				return
			case <-time.After(time.Duration(500+rand.Intn(1000)) * time.Millisecond):
				i := rand.Intn(nNodes)
				mu.Lock()
				if nodes[i] != nil {
					nodes[i].Stop()
					nodes[i] = nil
					mu.Unlock()

					time.Sleep(time.Duration(200+rand.Intn(500)) * time.Millisecond)

					mu.Lock()
					nodeDir := filepath.Join(tempDir, fmt.Sprintf("node-%d", i))
					node, err := StartNode(cfg, i, nodeDir, -1, false, true, "")
					if err == nil {
						nodes[i] = node
					}
				}
				mu.Unlock()
			}
		}
	}()
}

func TestLinearizability(t *testing.T) {
	nNodes := 3
	cfg, _, _ := setupTestCluster(t, nNodes, 10000, -1)

	addrs := make([]string, nNodes)
	for i := 0; i < nNodes; i++ {
		addrs[i] = cfg.Nodes[i].Addr
	}

	ck, err := kvclient.NewClerk(addrs)
	require.NoError(t, err)
	defer ck.Close()

	nClients, nOps := 5, 20
	history := make([]porcupine.Operation, nClients*nOps)
	historyMu, opIdx, wg := sync.Mutex{}, 0, sync.WaitGroup{}

	wg.Add(nClients)
	for c := 0; c < nClients; c++ {
		go runClientWorker(c, ck, nOps, "key", 50, history, &historyMu, &opIdx, &wg)
	}

	wg.Wait()
	path := getTimedVisualPath("linearizability")
	verifyLinearizability(t, history[:opIdx], path)
}

func TestSnapshotLaggingServer(t *testing.T) {
	nNodes, maxRaftState := 3, 1000
	cfg, nodes, tempDir := setupTestCluster(t, nNodes, 12000, maxRaftState)

	addrs := make([]string, nNodes)
	for i := 0; i < nNodes; i++ {
		addrs[i] = cfg.Nodes[i].Addr
	}

	ck, _ := kvclient.NewClerk(addrs)
	defer ck.Close()

	ck.Put("k1", "v1", 0)
	nodes[2].Stop()
	nodes[2] = nil

	for i := 0; i < 100; i++ {
		ck.Put("k1", fmt.Sprintf("v%d", i), api.TVersion(i+1))
	}

	nodeDir2 := filepath.Join(tempDir, "node-2")
	node2, err := StartNode(cfg, 2, nodeDir2, maxRaftState, false, true, "")
	require.NoError(t, err)
	nodes[2] = node2

	time.Sleep(2 * time.Second)
	val, ver, _ := ck.Get("k1")
	assert.Equal(t, "v99", val)
	assert.Equal(t, api.TVersion(101), ver)
}

func TestLinearizabilityStress(t *testing.T) {
	nNodes := 3
	cfg, nodes, tempDir := setupTestCluster(t, nNodes, 11000, -1)

	stopstress := make(chan struct{})
	startChaos(stopstress, nNodes, nodes, cfg, tempDir)
	defer close(stopstress)

	addrs := make([]string, nNodes)
	for i := 0; i < nNodes; i++ {
		addrs[i] = cfg.Nodes[i].Addr
	}

	ck, err := kvclient.NewClerk(addrs)
	require.NoError(t, err)
	defer ck.Close()

	nClients, nOps := 5, 30
	history := make([]porcupine.Operation, nClients*nOps)
	historyMu, opIdx, wg := sync.Mutex{}, 0, sync.WaitGroup{}

	wg.Add(nClients)
	for c := 0; c < nClients; c++ {
		go runClientWorker(c, ck, nOps, "stress-key", 100, history, &historyMu, &opIdx, &wg)
	}

	wg.Wait()
	path := getTimedVisualPath("linearizability-stress")
	verifyLinearizability(t, history[:opIdx], path)
}

func getTimedVisualPath(baseName string) string {
	timestamp := time.Now().Format("20060102-150405")
	return fmt.Sprintf("%s-%s.html", baseName, timestamp)
}

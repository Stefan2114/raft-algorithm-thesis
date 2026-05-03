package kvserver

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/anishathalye/porcupine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"kvraft/api"
	"kvraft/config"
	"kvraft/pkg/clerk"
)

func TestLinearizabilityChaos(t *testing.T) {
	nNodes := 3
	cfg := &config.Config{
		Nodes: make([]config.Node, nNodes),
	}
	for i := 0; i < nNodes; i++ {
		cfg.Nodes[i] = config.Node{
			ID:   i,
			Addr: fmt.Sprintf("localhost:%d", 11000+i),
		}
	}
	cfg.PopulateDefaults()

	tempDir, err := os.MkdirTemp("", "kvraft-chaos-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	nodes := make([]*Node, nNodes)
	nodeDirs := make([]string, nNodes)
	mu := sync.Mutex{} // Protects nodes slice during restarts

	for i := 0; i < nNodes; i++ {
		nodeDirs[i] = filepath.Join(tempDir, fmt.Sprintf("node-%d", i))
		node, err := StartNode(cfg, i, nodeDirs[i], -1, false, true, "")
		require.NoError(t, err, "failed to start node %d", i)
		nodes[i] = node
	}
	
	stopChaos := make(chan struct{})
	go func() {
		for {
			select {
			case <-stopChaos:
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
					node, err := StartNode(cfg, i, nodeDirs[i], -1, false, true, "")
					if err == nil {
						nodes[i] = node
					}
				}
				mu.Unlock()
			}
		}
	}()

	defer func() {
		close(stopChaos)
		mu.Lock()
		for _, n := range nodes {
			if n != nil {
				n.Stop()
			}
		}
		mu.Unlock()
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
	nOpsPerClient := 30
	var wg sync.WaitGroup
	wg.Add(nClients)

	history := make([]porcupine.Operation, nClients*nOpsPerClient)
	historyMu := sync.Mutex{}
	opIdx := 0

	for c := 0; c < nClients; c++ {
		go func(clientId int) {
			defer wg.Done()
			for i := 0; i < nOpsPerClient; i++ {
				key := "chaos-key"
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
				
				time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)
			}
		}(c)
	}

	wg.Wait()
	history = history[:opIdx]

	res, info := porcupine.CheckOperationsVerbose(kvModel, history, 0)
	if res == porcupine.Illegal {
		visualPath := "linearizability-chaos-failure.html"
		err := porcupine.VisualizePath(kvModel, info, visualPath)
		assert.NoError(t, err, "Failed to generate visualization")
		assert.Fail(t, "Linearizability check failed under chaos! Failure visualization saved to %s", visualPath)
	} else {
		t.Log("Linearizability check passed under chaos")
	}
}

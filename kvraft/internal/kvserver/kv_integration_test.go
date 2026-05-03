package kvserver

import (
	"github.com/stretchr/testify/assert"
	"kvraft/api"
	"kvraft/internal/logger"
	"kvraft/internal/raft"
	"kvraft/internal/rsm"
	"kvraft/raftapi"
	"kvraft/testutils"
	"kvraft/raftransport"
	"testing"
	"time"
	"encoding/gob"
	"kvraft/internal/models"
)

type kvCluster struct {
	net        *testutils.Network
	rsms       []*rsm.RSM
	rafts      []*raft.Raft
	persisters []raftapi.Persister
	stores     []*Store
	n          int
	opLog      *testutils.OpLog
}

func makeKVCluster(n int, maxRaftState int) *kvCluster {
	raftransport.RegisterRaftGobTypes()
	gob.Register("")
	net := testutils.NewNetwork()
	rsms := make([]*rsm.RSM, n)
	rafts := make([]*raft.Raft, n)
	stores := make([]*Store, n)
	persisters := make([]raftapi.Persister, n)

	for i := 0; i < n; i++ {
		persisters[i] = testutils.NewMockPersister()
		stores[i] = NewStore()
		
		cluster := &kvCluster{net: net, rsms: rsms, rafts: rafts, persisters: persisters, stores: stores, n: n}
		cluster.startNode(i, maxRaftState)
	}

	return &kvCluster{
		net:        net,
		rsms:       rsms,
		rafts:      rafts,
		persisters: persisters,
		stores:     stores,
		n:          n,
		opLog:      testutils.NewOpLog(),
	}
}

func (c *kvCluster) startNode(i int, maxRaftState int) {
	transportsInter := testutils.MakeMockTransports(c.net, i, c.n)
	transports := make([]raft.Transport, c.n)
	for j, ti := range transportsInter {
		transports[j] = ti.(raft.Transport)
	}

	l := logger.InitLogger(false, true, "")
	rsmInst := rsm.MakeRSM(transports, i, c.persisters[i], maxRaftState, c.stores[i], l,
		600*time.Millisecond, 400*time.Millisecond, 100*time.Millisecond, 10*time.Second)
	c.rsms[i] = rsmInst
	c.rafts[i] = rsmInst.Raft().(*raft.Raft)
	c.net.AddServer(i, c.rsms[i].Raft())
}

func (c *kvCluster) shutdownNode(i int) {
	c.rafts[i].Kill()
}

func (c *kvCluster) findLeader() int {
	for i, rf := range c.rafts {
		if _, isLeader := rf.GetState(); isLeader {
			return i
		}
	}
	return -1
}

func (c *kvCluster) submit(id int, req api.PutArgs, clientId int) (api.Err, api.PutReply) {
	start := time.Now()
	err, _ := c.rsms[id].Submit(req)
	end := time.Now()

	c.opLog.Append(
		models.KvInput{Op: 1, Key: req.Key, Value: req.Value, Version: uint64(req.Version)},
		models.KvOutput{Err: string(err)},
		start, end, clientId,
	)
	return err, api.PutReply{Err: err}
}

func (c *kvCluster) get(id int, req api.GetArgs, clientId int) (api.Err, api.GetReply) {
	start := time.Now()
	err, val := c.rsms[id].Submit(req)
	end := time.Now()

	reply := api.GetReply{Err: err}
	if err == api.OK {
		// In our RSM, Submit returns the value for Get too
		if v, ok := val.(string); ok {
			reply.Value = v
		}
	}

	c.opLog.Append(
		models.KvInput{Op: 0, Key: req.Key},
		models.KvOutput{Value: reply.Value, Err: string(err)},
		start, end, clientId,
	)
	return err, reply
}

func TestKV_Partition(t *testing.T) {
	c := makeKVCluster(3, -1)
	
	// Wait for leader
	leader := -1
	for i := 0; i < 10; i++ {
		leader = c.findLeader()
		if leader != -1 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	assert.NotEqual(t, -1, leader, "no leader elected")
	defer c.opLog.Check(t)

	// 1. Progress in majority
	req1 := api.PutArgs{Key: "k1", Value: "v1", Version: 0}
	err1, _ := c.submit(leader, req1, 0)
	assert.Equal(t, api.OK, err1, "Put failed")

	// 2. Partition leader
	other1 := (leader + 1) % 3
	other2 := (leader + 2) % 3
	c.net.Disconnect(leader, other1)
	c.net.Disconnect(leader, other2)
	c.net.Disconnect(other1, leader)
	c.net.Disconnect(other2, leader)

	// 3. Submit to minority (leader), should fail or timeout
	req2 := api.PutArgs{Key: "k1", Value: "v2", Version: 1}
	done := make(chan bool)
	go func() {
		err, _ := c.submit(leader, req2, 0)
		if err != api.OK {
			done <- true
		} else {
			done <- false
		}
	}()

	select {
	case success := <-done:
		assert.True(t, success, "Put in minority should have failed or timed out")
	case <-time.After(1 * time.Second):
		// Expected timeout
	}

	// 4. Other partition should elect new leader and progress
	newLeader := -1
	for i := 0; i < 20; i++ {
		if _, isLeader := c.rafts[other1].GetState(); isLeader {
			newLeader = other1
			break
		}
		if _, isLeader := c.rafts[other2].GetState(); isLeader {
			newLeader = other2
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	assert.NotEqual(t, -1, newLeader, "new leader not elected in majority partition")

	req3 := api.PutArgs{Key: "k1", Value: "v3", Version: 1}
	err3, _ := c.submit(newLeader, req3, 0)
	assert.Equal(t, api.OK, err3, "Put in majority failed")

	// 5. Heal partition
	c.net.Connect(leader, other1)
	c.net.Connect(leader, other2)
	c.net.Connect(other1, leader)
	c.net.Connect(other2, leader)

	time.Sleep(1 * time.Second)

	// 6. Verify progress continues
	req4 := api.PutArgs{Key: "k1", Value: "v4", Version: 2}
	err4, _ := c.submit(newLeader, req4, 0)
	assert.Equal(t, api.OK, err4, "Put after heal failed")
}

func TestKV_UnreliableNetwork(t *testing.T) {
	c := makeKVCluster(3, -1)
	defer c.opLog.Check(t)
	c.net.SetReliability(false)

	// Submit some requests
	for i := 0; i < 20; i++ {
		req := api.PutArgs{Key: "k", Value: string(rune('a' + i)), Version: api.TVersion(i)}
		// We might need to retry since network is unreliable
		success := false
		for try := 0; try < 10; try++ {
			leader := c.findLeader()
			if leader != -1 {
				err, _ := c.submit(leader, req, 0)
				if err == api.OK {
					success = true
					break
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
		assert.True(t, success, "Failed to submit request %d even after retries", i)
	}
}

func TestKV_SnapshotRPC(t *testing.T) {
	// Small maxRaftState to trigger snapshots
	c := makeKVCluster(3, 1000)
	defer c.opLog.Check(t)
	
	leader := -1
	for i := 0; i < 10; i++ {
		leader = c.findLeader()
		if leader != -1 { break }
		time.Sleep(200 * time.Millisecond)
	}

	// 1. Partition one follower
	follower := (leader + 1) % 3
	c.net.Disconnect(follower, leader)
	c.net.Disconnect(follower, (leader+2)%3)
	c.net.Disconnect(leader, follower)
	c.net.Disconnect((leader+2)%3, follower)

	// 2. Send many requests to majority to trigger snapshots
	for i := 0; i < 100; i++ {
		req := api.PutArgs{Key: "k", Value: testutils.RandString(100), Version: api.TVersion(i)}
		c.submit(leader, req, 0)
	}

	// 3. Reconnect follower
	c.net.Connect(follower, leader)
	c.net.Connect(follower, (leader+2)%3)
	c.net.Connect(leader, follower)
	c.net.Connect((leader+2)%3, follower)

	// 4. Wait for follower to catch up via snapshot
	time.Sleep(2 * time.Second)
	
	// 5. Verify follower has the state
	applied := c.rsms[follower].LastApplied()
	assert.GreaterOrEqual(t, applied, 100, "Follower did not catch up via snapshot")
}

func TestKV_CrashRecovery(t *testing.T) {
	c := makeKVCluster(3, -1)
	defer c.opLog.Check(t)
	
	// 1. Wait for initial leader
	leader := -1
	for i := 0; i < 10; i++ {
		leader = c.findLeader()
		if leader != -1 { break }
		time.Sleep(200 * time.Millisecond)
	}
	assert.NotEqual(t, -1, leader, "Initial leader not elected")

	// 2. Submit some requests
	for i := 0; i < 10; i++ {
		req := api.PutArgs{Key: "k", Value: string(rune('0' + i)), Version: api.TVersion(i)}
		err, _ := c.submit(leader, req, 0)
		if err != api.OK {
			// retry if leader changed
			leader = c.findLeader()
			if leader != -1 {
				c.submit(leader, req, 0)
			}
		}
	}

	// 2. Kill all nodes
	for i := 0; i < 3; i++ {
		c.shutdownNode(i)
	}
	
	time.Sleep(500 * time.Millisecond)

	// 3. Restart nodes
	for i := 0; i < 3; i++ {
		c.startNode(i, -1)
	}

	// 4. Wait for election
	time.Sleep(1 * time.Second)

	// 5. Verify data is still there
	leader = -1
	for i := 0; i < 10; i++ {
		leader = c.findLeader()
		if leader != -1 { break }
		time.Sleep(200 * time.Millisecond)
	}
	
	assert.NotEqual(t, -1, leader, "No leader after restart")

	// Submit one more to check stability
	req := api.PutArgs{Key: "k", Value: "final", Version: api.TVersion(10)}
	err, _ := c.submit(leader, req, 0)
	assert.Equal(t, api.OK, err, "Failed to submit request after recovery")
}

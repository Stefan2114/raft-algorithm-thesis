package kvserver

import (
	"encoding/gob"
	"kvraft/api"
	"kvraft/internal/logger"
	"kvraft/internal/rsm"
	"kvraft/internal/testutils"
	"kvraft/raft"
	"kvraft/raftransport"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type kvCluster struct {
	net        *testutils.Network
	rsms       []*rsm.RSM
	rafts      []raft.Raft
	persisters []raft.Persister
	stores     []*Store
	n          int
	opLog      *testutils.OpLog
}

func makeKVCluster(n int, maxRaftState int) *kvCluster {
	raftransport.RegisterRaftGobTypes()
	gob.Register("")
	net := testutils.NewNetwork()
	rsms := make([]*rsm.RSM, n)
	rafts := make([]raft.Raft, n)
	stores := make([]*Store, n)
	persisters := make([]raft.Persister, n)

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
	c.rafts[i] = rsmInst.Raft()
	c.net.AddServer(i, c.rsms[i].Raft())
}

func (c *kvCluster) shutdownNode(i int) {
	c.rafts[i].Kill()
}

func (c *kvCluster) findLeader() int {
	for i, rf := range c.rafts {
		if _, isLeader := rf.State(); isLeader {
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
		testutils.KvInput{Op: 1, Key: req.Key, Value: req.Value, Version: uint64(req.Version)},
		testutils.KvOutput{Err: string(err)},
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
		if v, ok := val.(string); ok {
			reply.Value = v
		}
	}

	c.opLog.Append(
		testutils.KvInput{Op: 0, Key: req.Key},
		testutils.KvOutput{Value: reply.Value, Err: string(err)},
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

	req1 := api.PutArgs{Key: "k1", Value: "v1", Version: 0}
	err1, _ := c.submit(leader, req1, 0)
	assert.Equal(t, api.OK, err1, "Put failed")

	// Partition leader
	other1 := (leader + 1) % 3
	other2 := (leader + 2) % 3
	c.net.Disconnect(leader, other1)
	c.net.Disconnect(leader, other2)
	c.net.Disconnect(other1, leader)
	c.net.Disconnect(other2, leader)

	// Submit to minority (leader), should fail or timeout
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

	// Other partition should elect new leader and progress
	newLeader := -1
	for i := 0; i < 20; i++ {
		if _, isLeader := c.rafts[other1].State(); isLeader {
			newLeader = other1
			break
		}
		if _, isLeader := c.rafts[other2].State(); isLeader {
			newLeader = other2
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	assert.NotEqual(t, -1, newLeader, "new leader not elected in majority partition")

	req3 := api.PutArgs{Key: "k1", Value: "v3", Version: 1}
	err3, _ := c.submit(newLeader, req3, 0)
	assert.Equal(t, api.OK, err3, "Put in majority failed")

	// Heal partition
	c.net.Connect(leader, other1)
	c.net.Connect(leader, other2)
	c.net.Connect(other1, leader)
	c.net.Connect(other2, leader)

	time.Sleep(1 * time.Second)

	// Verify progress continues
	req4 := api.PutArgs{Key: "k1", Value: "v4", Version: 2}
	err4, _ := c.submit(newLeader, req4, 0)
	assert.Equal(t, api.OK, err4, "Put after heal failed")
}

func TestKV_UnreliableNetwork(t *testing.T) {
	c := makeKVCluster(3, -1)
	defer c.opLog.Check(t)
	c.net.SetReliability(false)

	for i := 0; i < 20; i++ {
		req := api.PutArgs{Key: "k", Value: string(rune('a' + i)), Version: api.TVersion(i)}
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
	c := makeKVCluster(3, 1000)
	defer c.opLog.Check(t)

	leader := -1
	for i := 0; i < 10; i++ {
		leader = c.findLeader()
		if leader != -1 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	follower := (leader + 1) % 3
	c.net.Disconnect(follower, leader)
	c.net.Disconnect(follower, (leader+2)%3)
	c.net.Disconnect(leader, follower)
	c.net.Disconnect((leader+2)%3, follower)

	for i := 0; i < 100; i++ {
		req := api.PutArgs{Key: "k", Value: testutils.RandString(100), Version: api.TVersion(i)}
		c.submit(leader, req, 0)
	}

	c.net.Connect(follower, leader)
	c.net.Connect(follower, (leader+2)%3)
	c.net.Connect(leader, follower)
	c.net.Connect((leader+2)%3, follower)

	time.Sleep(2 * time.Second)

	applied := c.rsms[follower].LastApplied()
	assert.GreaterOrEqual(t, applied, 100, "Follower did not catch up via snapshot")
}

func TestKV_CrashRecovery(t *testing.T) {
	c := makeKVCluster(3, -1)
	defer c.opLog.Check(t)

	leader := -1
	for i := 0; i < 10; i++ {
		leader = c.findLeader()
		if leader != -1 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	assert.NotEqual(t, -1, leader, "Initial leader not elected")

	for i := 0; i < 10; i++ {
		req := api.PutArgs{Key: "k", Value: string(rune('0' + i)), Version: api.TVersion(i)}
		err, _ := c.submit(leader, req, 0)
		if err != api.OK {
			leader = c.findLeader()
			if leader != -1 {
				c.submit(leader, req, 0)
			}
		}
	}

	for i := 0; i < 3; i++ {
		c.shutdownNode(i)
	}

	time.Sleep(500 * time.Millisecond)
	for i := 0; i < 3; i++ {
		c.startNode(i, -1)
	}
	time.Sleep(1 * time.Second)
	leader = -1
	for i := 0; i < 10; i++ {
		leader = c.findLeader()
		if leader != -1 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	assert.NotEqual(t, -1, leader, "No leader after restart")
	req := api.PutArgs{Key: "k", Value: "final", Version: api.TVersion(10)}
	err, _ := c.submit(leader, req, 0)
	assert.Equal(t, api.OK, err, "Failed to submit request after recovery")
}

package raft

import (
	"kvraft/internal/logger"
	"kvraft/raftapi"
	"kvraft/testutils"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func makeCluster(n int) (*testutils.Network, []*Raft, []raftapi.Persister) {
	net := testutils.NewNetwork()
	rafts := make([]*Raft, n)
	persisters := make([]raftapi.Persister, n)

	for i := 0; i < n; i++ {
		persisters[i] = testutils.NewMockPersister()
		rafts[i] = startRaft(i, n, net, persisters[i])
		net.AddServer(i, rafts[i])
	}

	return net, rafts, persisters
}

func startRaft(me int, n int, net *testutils.Network, persister raftapi.Persister) *Raft {
	transportsInter := testutils.MakeMockTransports(net, me, n)
	transports := make([]Transport, n)
	for j, ti := range transportsInter {
		transports[j] = ti.(Transport)
	}

	applyCh := make(chan raftapi.ApplyMsg, 1000)
	l := logger.InitLogger(false, true, "")

	rfInter := Make(transports, me, persister, applyCh, l,
		600*time.Millisecond, 400*time.Millisecond, 100*time.Millisecond)
	return rfInter.(*Raft)
}

func checkOneLeader(t *testing.T, rafts []*Raft) int {
	for iters := 0; iters < 10; iters++ {
		ms := 50 + (iters * 100)
		time.Sleep(time.Duration(ms) * time.Millisecond)

		leaders := make(map[int][]int)
		for i, rf := range rafts {
			if term, isLeader := rf.GetState(); isLeader {
				leaders[term] = append(leaders[term], i)
			}
		}

		lastTermWithLeader := -1
		for term, leaderList := range leaders {
			if len(leaderList) > 1 {
				t.Fatalf("term %d has multiple leaders: %v", term, leaderList)
			}
			if term > lastTermWithLeader {
				lastTermWithLeader = term
			}
		}

		if len(leaders) != 0 {
			t.Logf("Found leaders: %v", leaders)
			return leaders[lastTermWithLeader][0]
		}
	}
	t.Fatalf("expected one leader, got none")
	return -1
}

func waitApplied(t *testing.T, index int, expectedCount int, rafts []*Raft, timeout time.Duration) int {
	start := time.Now()
	for time.Since(start) < timeout {
		count := 0
		for _, rf := range rafts {
			rf.mu.RLock()
			applied := rf.lastApplied >= index
			rf.mu.RUnlock()
			if applied {
				count++
			}
		}
		if count >= expectedCount {
			return count
		}
		time.Sleep(20 * time.Millisecond)
	}
	return 0
}

func TestRaft_InitialElection(t *testing.T) {
	_, rafts, _ := makeCluster(3)
	defer func() {
		for _, rf := range rafts {
			rf.Kill()
		}
	}()

	leader := checkOneLeader(t, rafts)
	assert.GreaterOrEqual(t, leader, 0, "invalid leader index")
	assert.LessOrEqual(t, leader, 2, "invalid leader index")
}

func TestRaft_ReElection(t *testing.T) {
	net, rafts, _ := makeCluster(3)
	defer func() {
		for _, rf := range rafts {
			rf.Kill()
		}
	}()

	leader1 := checkOneLeader(t, rafts)

	for i := 0; i < 3; i++ {
		net.Disconnect(leader1, i)
	}

	leader2 := -1
	for iters := 0; iters < 20; iters++ {
		time.Sleep(200 * time.Millisecond)
		leader2 = checkOneLeader(t, rafts)
		if leader2 != leader1 {
			break
		}
	}

	assert.NotEqual(t, leader1, leader2, "expected new leader, but got the same leader")

	// Reconnect the old leader
	for i := 0; i < 3; i++ {
		net.Connect(leader1, i)
	}

	leader3 := checkOneLeader(t, rafts)
	assert.NotEqual(t, leader1, leader3, "old leader became leader again, but it should have a lower term")
}

func TestRaft_BasicAppend(t *testing.T) {
	_, rafts, _ := makeCluster(3)
	defer func() {
		for _, rf := range rafts {
			rf.Kill()
		}
	}()

	leader := checkOneLeader(t, rafts)

	cmd := "test_command"
	index, _, isLeader := rafts[leader].Start(cmd)
	assert.True(t, isLeader, "Start failed, node %d is not leader", leader)

	applied := waitApplied(t, index, 3, rafts, 2*time.Second)
	assert.Equal(t, 3, applied, "Only %d nodes applied command at index %d, expected 3", applied, index)

	for i, rf := range rafts {
		select {
		case msg := <-rf.applyCh:
			assert.True(t, msg.CommandValid, "Expected valid command msg")
			assert.Equal(t, index, msg.CommandIndex, "Expected index %v, got %v", index, msg.CommandIndex)
			assert.Equal(t, cmd, msg.Command, "Expected cmd %v, got %v", cmd, msg.Command)
		default:
			assert.Fail(t, "Node %d did not have command in applyCh despite waitApplied", i)
		}
	}
}
func TestRaft_Persist(t *testing.T) {
	net, rafts, persisters := makeCluster(3)
	defer func() {
		for _, rf := range rafts {
			rf.Kill()
		}
	}()

	leader := checkOneLeader(t, rafts)
	rafts[leader].Start("cmd1")
	time.Sleep(500 * time.Millisecond)

	for i := 0; i < 3; i++ {
		rafts[i].Kill()
		rafts[i] = startRaft(i, 3, net, persisters[i])
		net.AddServer(i, rafts[i])
	}

	newLeader := checkOneLeader(t, rafts)
	rafts[newLeader].Start("cmd2")
	time.Sleep(500 * time.Millisecond)

	for i := 0; i < 3; i++ {
		select {
		case msg := <-rafts[i].applyCh:
			assert.Equal(t, "cmd1", msg.Command, "Expected cmd1")
		default:
			assert.Fail(t, "Node %d lost cmd1 after restart", i)
		}
	}
}

func TestRaft_Backup(t *testing.T) {
	net, rafts, _ := makeCluster(5)
	defer func() {
		for _, rf := range rafts {
			rf.Kill()
		}
	}()

	leader1 := checkOneLeader(t, rafts)

	other1 := (leader1 + 1) % 5
	for i := 0; i < 5; i++ {
		if i != leader1 && i != other1 {
			net.Disconnect(leader1, i)
			net.Disconnect(other1, i)
		}
	}

	for i := 0; i < 50; i++ {
		rafts[leader1].Start(i)
	}
	time.Sleep(200 * time.Millisecond)

	for i := 0; i < 5; i++ {
		for j := 0; j < 5; j++ {
			net.Disconnect(i, j)
		}
	}
	p2 := []int{}
	for i := 0; i < 5; i++ {
		if i != leader1 && i != other1 {
			p2 = append(p2, i)
		}
	}
	for _, i := range p2 {
		for _, j := range p2 {
			net.Connect(i, j)
		}
	}

	leader2 := checkOneLeader(t, rafts)
	for i := 0; i < 50; i++ {
		rafts[leader2].Start(i + 100)
	}
	time.Sleep(500 * time.Millisecond)

	for i := 0; i < 5; i++ {
		for j := 0; j < 5; j++ {
			net.Connect(i, j)
		}
	}

	leader3 := checkOneLeader(t, rafts)
	index999, _, _ := rafts[leader3].Start(999)
	applied := waitApplied(t, index999, 5, rafts, 2*time.Second)
	assert.Equal(t, 5, applied, "Only %d nodes applied final command, expected 5", applied)

	for _, rf := range rafts {
		rf.mu.RLock()
		for _, entry := range rf.logs {
			assert.NotEqual(t, 0, entry.Command, "Old command 0 found in node %d log", rf.me)
			assert.NotEqual(t, 49, entry.Command, "Old command 49 found in node %d log", rf.me)
		}
		rf.mu.RUnlock()
	}
}

func TestRaft_ManyElections(t *testing.T) {
	n := 7
	net, rafts, _ := makeCluster(n)
	defer func() {
		for _, rf := range rafts {
			rf.Kill()
		}
	}()

	checkOneLeader(t, rafts)

	iters := 10
	for ii := 1; ii < iters; ii++ {
		i1 := rand.Int() % n
		i2 := rand.Int() % n
		i3 := rand.Int() % n

		for j := 0; j < n; j++ {
			net.Disconnect(i1, j)
			net.Disconnect(i2, j)
			net.Disconnect(i3, j)
			net.Disconnect(j, i1)
			net.Disconnect(j, i2)
			net.Disconnect(j, i3)
		}

		checkOneLeader(t, rafts)

		for j := 0; j < n; j++ {
			net.Connect(i1, j)
			net.Connect(i2, j)
			net.Connect(i3, j)
			net.Connect(j, i1)
			net.Connect(j, i2)
			net.Connect(j, i3)
		}
	}
	checkOneLeader(t, rafts)
}

func TestRaft_ConcurrentStarts(t *testing.T) {
	_, rafts, _ := makeCluster(3)
	defer func() {
		for _, rf := range rafts {
			rf.Kill()
		}
	}()

	leader := checkOneLeader(t, rafts)

	iters := 10
	var wg sync.WaitGroup
	indices := make(chan int, iters)

	for i := 0; i < iters; i++ {
		wg.Add(1)
		go func(cmd int) {
			defer wg.Done()
			index, _, isLeader := rafts[leader].Start(cmd)
			if isLeader {
				indices <- index
			}
		}(i)
	}

	wg.Wait()
	close(indices)

	for index := range indices {
		applied := waitApplied(t, index, 3, rafts, 2*time.Second)
		assert.Equal(t, 3, applied, "Command at index %d not applied by all nodes", index)
	}
}

func TestRaft_RPCBytes(t *testing.T) {
	net, rafts, _ := makeCluster(3)
	defer func() {
		for _, rf := range rafts {
			rf.Kill()
		}
	}()

	leader := checkOneLeader(t, rafts)

	rafts[leader].Start("init")
	time.Sleep(200 * time.Millisecond)

	bytes0 := 0
	for i := 0; i < 3; i++ {
		bytes0 += net.GetRPCCount(i)
	}

	iters := 10
	for i := 0; i < iters; i++ {
		rafts[leader].Start(testutils.RandString(1000))
	}

	time.Sleep(500 * time.Millisecond)

	bytes1 := 0
	for i := 0; i < 3; i++ {
		bytes1 += net.GetRPCCount(i)
	}

	got := bytes1 - bytes0
	expectedMax := (iters + 5) * 3
	assert.LessOrEqual(t, got, expectedMax, "Too many RPCs; got %v, expected max %v", got, expectedMax)
}

func TestRaft_CountRPC(t *testing.T) {
	net, rafts, _ := makeCluster(3)
	defer func() {
		for _, rf := range rafts {
			rf.Kill()
		}
	}()

	checkOneLeader(t, rafts)
	net.ResetRPCCounts()

	time.Sleep(1 * time.Second)

	total := 0
	for i := 0; i < 3; i++ {
		total += net.GetRPCCount(i)
	}

	assert.LessOrEqual(t, total, 50, "Too many RPCs in idle; got %v, expected < 50", total)
}

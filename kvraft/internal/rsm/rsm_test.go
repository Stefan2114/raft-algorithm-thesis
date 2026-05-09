package rsm

import (
	"kvraft/api"
	"kvraft/internal/logger"
	"kvraft/internal/testutils"
	"kvraft/raft"
	"kvraft/sm"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRSM_SubmitSuccess(t *testing.T) {
	// Prevent MakeRSM from instantiating the real Raft
	useRaftStateMachine = true
	defer func() { useRaftStateMachine = false }()

	l := logger.InitLogger(false, true, "")
	mock_sm := testutils.NewMockStateMachine()
	persister := testutils.NewMockPersister()

	rsm := MakeRSM(nil, 0, persister, -1, mock_sm, l, 600*time.Millisecond, 400*time.Millisecond, 100*time.Millisecond, 10*time.Second)

	mockRaft := testutils.NewMockRaft(0, rsm.applyCh, persister)
	rsm.rf = mockRaft

	req := "test_command"

	errCh := make(chan api.Err)
	valCh := make(chan any)

	go func() {
		err, val := rsm.Submit(req)
		errCh <- err
		valCh <- val
	}()

	select {
	case err := <-errCh:
		val := <-valCh
		assert.Equal(t, api.OK, err, "Expected OK")
		assert.Equal(t, req, val, "Expected result %v, got %v", req, val)
	case <-time.After(2 * time.Second):
		assert.Fail(t, "Submit timed out")
	}

	assert.Equal(t, 1, mock_sm.GetAppliedCount(), "Expected 1 applied command")
}

func TestRSM_SubmitNotLeader(t *testing.T) {
	useRaftStateMachine = true
	defer func() { useRaftStateMachine = false }()

	l := logger.InitLogger(false, true, "")
	mock_sm := testutils.NewMockStateMachine()
	persister := testutils.NewMockPersister()

	rsm := MakeRSM(nil, 0, persister, -1, mock_sm, l, 600*time.Millisecond, 400*time.Millisecond, 100*time.Millisecond, 10*time.Second)
	mockRaft := testutils.NewMockRaft(0, rsm.applyCh, persister)
	mockRaft.SetLeader(false, 1) // Not leader, leader is 1
	rsm.rf = mockRaft

	err, leaderId := rsm.Submit("test_command")
	assert.Equal(t, api.ErrWrongLeader, err, "Expected ErrWrongLeader")
	assert.Equal(t, 1, leaderId, "Expected leader id 1")
}

func TestRSM_SnapshotTrigger(t *testing.T) {
	useRaftStateMachine = true
	defer func() { useRaftStateMachine = false }()

	l := logger.InitLogger(false, true, "")
	mock_sm := testutils.NewMockStateMachine()
	persister := testutils.NewMockPersister()

	rsm := MakeRSM(nil, 0, persister, 10, mock_sm, l, 600*time.Millisecond, 400*time.Millisecond, 100*time.Millisecond, 10*time.Second)
	mockRaft := testutils.NewMockRaft(0, rsm.applyCh, persister)
	rsm.rf = mockRaft

	persister.Save(make([]byte, 20), nil)

	err, _ := rsm.Submit("test_command")
	assert.Equal(t, api.OK, err, "Expected OK")

	time.Sleep(100 * time.Millisecond)

	assert.NotZero(t, persister.SnapshotSize(), "Expected snapshot to be taken")
}

func TestRSM_RestoreFromSnapshot(t *testing.T) {
	useRaftStateMachine = true
	defer func() { useRaftStateMachine = false }()

	l := logger.InitLogger(false, true, "")
	mock_sm := testutils.NewMockStateMachine()
	persister := testutils.NewMockPersister()

	rsm1 := MakeRSM(nil, 0, persister, 10, mock_sm, l, 600*time.Millisecond, 400*time.Millisecond, 100*time.Millisecond, 10*time.Second)
	mockRaft := testutils.NewMockRaft(0, rsm1.applyCh, persister)
	rsm1.rf = mockRaft
	persister.Save(make([]byte, 20), nil)
	rsm1.Submit("command_1")

	time.Sleep(100 * time.Millisecond)

	sm2 := testutils.NewMockStateMachine()
	MakeRSM(nil, 0, persister, 10, sm2, l, 600*time.Millisecond, 400*time.Millisecond, 100*time.Millisecond, 10*time.Second)

	assert.Equal(t, 1, sm2.GetAppliedCount(), "Expected state machine to be restored with 1 applied command")
}

func TestRSM_LeaderChangeDuringSubmit(t *testing.T) {
	useRaftStateMachine = true
	defer func() { useRaftStateMachine = false }()

	l := logger.InitLogger(false, true, "")
	mock_sm := testutils.NewMockStateMachine()
	persister := testutils.NewMockPersister()

	rsm := MakeRSM(nil, 0, persister, -1, mock_sm, l, 600*time.Millisecond, 400*time.Millisecond, 100*time.Millisecond, 10*time.Second)
	dummyCh := make(chan raft.ApplyMsg, 10)
	mockRaft := testutils.NewMockRaft(0, dummyCh, persister)
	rsm.rf = mockRaft

	errCh := make(chan api.Err)
	go func() {
		err, _ := rsm.Submit("pending_command")
		errCh <- err
	}()

	time.Sleep(50 * time.Millisecond)

	// Simulate leader change by applying an empty log or another term
	mockRaft.SetLeader(false, 1)

	rsm.applyCh <- raft.ApplyMsg{
		CommandValid: true,
		Command:      sm.Op{Id: 999, Req: "different_command"},
		CommandIndex: 1, // Same index
	}

	select {
	case err := <-errCh:
		assert.Equal(t, api.ErrWrongLeader, err, "Expected ErrWrongLeader due to term mismatch")
	case <-time.After(1 * time.Second):
		assert.Fail(t, "Timeout waiting for leader change resolution")
	}
}

func TestRSM_SnapshotIsolation(t *testing.T) {
	useRaftStateMachine = true
	defer func() { useRaftStateMachine = false }()

	l := logger.InitLogger(false, true, "")
	mock_sm := testutils.NewMockStateMachine()
	persister := testutils.NewMockPersister()

	rsm := MakeRSM(nil, 0, persister, 10, mock_sm, l, 600*time.Millisecond, 400*time.Millisecond, 100*time.Millisecond, 10*time.Second)
	mockRaft := testutils.NewMockRaft(0, rsm.applyCh, persister)
	rsm.rf = mockRaft

	iters := 50
	errCh := make(chan api.Err, iters)
	for i := 0; i < iters; i++ {
		go func(cmd int) {
			persister.Save(make([]byte, 20), nil)
			err, _ := rsm.Submit(cmd)
			errCh <- err
		}(i)
	}

	for i := 0; i < iters; i++ {
		select {
		case err := <-errCh:
			assert.Equal(t, api.OK, err, "Command %d failed", i)
		case <-time.After(5 * time.Second):
			assert.Fail(t, "Timed out waiting for command %d", i)
		}
	}

	assert.Equal(t, iters, mock_sm.GetAppliedCount(), "Expected %d applied commands", iters)
}

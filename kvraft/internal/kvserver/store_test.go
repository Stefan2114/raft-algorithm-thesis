package kvserver

import (
	"kvraft/api"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStore_GetEmpty(t *testing.T) {
	store := NewStore()
	res := store.DoOp(api.GetArgs{Key: "foo"})
	getRes := res.(api.GetReply)
	assert.Equal(t, api.ErrNoKey, getRes.Err)
}

func TestStore_PutNew(t *testing.T) {
	store := NewStore()
	res := store.DoOp(api.PutArgs{Key: "foo", Value: "bar", Version: 0})
	putRes := res.(api.PutReply)
	assert.Equal(t, api.OK, putRes.Err)

	res = store.DoOp(api.GetArgs{Key: "foo"})
	getRes := res.(api.GetReply)
	assert.Equal(t, api.OK, getRes.Err)
	assert.Equal(t, "bar", getRes.Value)
	assert.Equal(t, api.TVersion(1), getRes.Version)
}

func TestStore_UpdateExisting(t *testing.T) {
	store := NewStore()
	store.DoOp(api.PutArgs{Key: "foo", Value: "bar", Version: 0})

	res := store.DoOp(api.PutArgs{Key: "foo", Value: "baz", Version: 1})
	putRes := res.(api.PutReply)
	assert.Equal(t, api.OK, putRes.Err)

	res = store.DoOp(api.GetArgs{Key: "foo"})
	getRes := res.(api.GetReply)
	assert.Equal(t, "baz", getRes.Value)
	assert.Equal(t, api.TVersion(2), getRes.Version)
}

func TestStore_InvalidVersion(t *testing.T) {
	store := NewStore()
	store.DoOp(api.PutArgs{Key: "foo", Value: "bar", Version: 0})

	// Version too high
	res := store.DoOp(api.PutArgs{Key: "foo", Value: "qux", Version: 5})
	assert.Equal(t, api.ErrVersion, res.(api.PutReply).Err)

	// Version too low
	res = store.DoOp(api.PutArgs{Key: "foo", Value: "qux", Version: 0})
	assert.Equal(t, api.ErrVersion, res.(api.PutReply).Err)
}

func TestStore_SnapshotRestore(t *testing.T) {
	store1 := NewStore()
	store1.DoOp(api.PutArgs{Key: "k1", Value: "v1", Version: 0})
	store1.DoOp(api.PutArgs{Key: "k2", Value: "v2", Version: 0})
	store1.DoOp(api.PutArgs{Key: "k1", Value: "v1-2", Version: 1})

	snap, err := store1.Snapshot()
	assert.NoError(t, err)

	store2 := NewStore()
	err = store2.Restore(snap)
	assert.NoError(t, err)

	res := store2.DoOp(api.GetArgs{Key: "k1"})
	getRes := res.(api.GetReply)
	assert.Equal(t, api.OK, getRes.Err)
	assert.Equal(t, "v1-2", getRes.Value)
	assert.Equal(t, api.TVersion(2), getRes.Version)

	res = store2.DoOp(api.GetArgs{Key: "k2"})
	getRes = res.(api.GetReply)
	assert.Equal(t, api.OK, getRes.Err)
	assert.Equal(t, "v2", getRes.Value)
	assert.Equal(t, api.TVersion(1), getRes.Version)
}

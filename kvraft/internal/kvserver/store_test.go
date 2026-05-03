package kvserver

import (
	"github.com/stretchr/testify/assert"
	"kvraft/api"
	"testing"
)

func TestStore_GetPut(t *testing.T) {
	store := NewStore()

	// Test Get on empty store
	res := store.DoOp(api.GetArgs{Key: "foo"})
	getRes := res.(api.GetReply)
	assert.Equal(t, api.ErrNoKey, getRes.Err, "Expected ErrNoKey")

	// Test Put new key
	res = store.DoOp(api.PutArgs{Key: "foo", Value: "bar", Version: 0})
	putRes := res.(api.PutReply)
	assert.Equal(t, api.OK, putRes.Err, "Expected OK")

	// Test Get existing key
	res = store.DoOp(api.GetArgs{Key: "foo"})
	getRes = res.(api.GetReply)
	assert.Equal(t, api.OK, getRes.Err)
	assert.Equal(t, "bar", getRes.Value)
	assert.Equal(t, api.TVersion(1), getRes.Version)

	// Test Put existing key with correct version
	res = store.DoOp(api.PutArgs{Key: "foo", Value: "baz", Version: 1})
	putRes = res.(api.PutReply)
	assert.Equal(t, api.OK, putRes.Err, "Expected OK")

	// Verify updated value and version
	res = store.DoOp(api.GetArgs{Key: "foo"})
	getRes = res.(api.GetReply)
	assert.Equal(t, api.OK, getRes.Err)
	assert.Equal(t, "baz", getRes.Value)
	assert.Equal(t, api.TVersion(2), getRes.Version)

	// Test Put existing key with incorrect version (too high)
	res = store.DoOp(api.PutArgs{Key: "foo", Value: "qux", Version: 5})
	putRes = res.(api.PutReply)
	assert.Equal(t, api.ErrVersion, putRes.Err, "Expected ErrVersion")

	// Test Put existing key with incorrect version (too low / old)
	res = store.DoOp(api.PutArgs{Key: "foo", Value: "qux", Version: 1})
	putRes = res.(api.PutReply)
	assert.Equal(t, api.ErrVersion, putRes.Err, "Expected ErrVersion")
}

func TestStore_SnapshotRestore(t *testing.T) {
	store1 := NewStore()
	store1.DoOp(api.PutArgs{Key: "k1", Value: "v1", Version: 0})
	store1.DoOp(api.PutArgs{Key: "k2", Value: "v2", Version: 0})
	store1.DoOp(api.PutArgs{Key: "k1", Value: "v1-2", Version: 1})

	snap := store1.Snapshot()

	store2 := NewStore()
	store2.Restore(snap)

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

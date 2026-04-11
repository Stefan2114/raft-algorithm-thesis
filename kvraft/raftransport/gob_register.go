package raftransport

import (
	"encoding/gob"

	"kvraft/api"
	"kvraft/internal/rsm"
)

// RegisterRaftGobTypes registers types that appear in replicated log entries.
func RegisterRaftGobTypes() {
	gob.Register(rsm.Op{})
	gob.Register(api.GetArgs{})
	gob.Register(api.PutArgs{})
	gob.Register(api.GetReply{})
	gob.Register(api.PutReply{})
	gob.Register(int(0))
}

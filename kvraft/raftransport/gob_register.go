package raftransport

import (
	"encoding/gob"
	"kvraft/sm"

	"kvraft/api"
)

func RegisterRaftGobTypes() {
	gob.Register(sm.Op{})
	gob.Register(api.GetArgs{})
	gob.Register(api.PutArgs{})
	gob.Register(api.GetReply{})
	gob.Register(api.PutReply{})
	gob.Register(int(0))
}

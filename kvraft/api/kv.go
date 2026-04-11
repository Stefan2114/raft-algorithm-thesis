// Package api holds shared KV request/response types and errors used by the
// state machine, tests, and gRPC. Keeping this small avoids rsm importing
// a heavy server package.
package api

type Err string

const (
	OK             Err = "OK"
	ErrNoKey       Err = "ErrNoKey"
	ErrVersion     Err = "ErrVersion"
	ErrMaybe       Err = "ErrMaybe"
	ErrWrongLeader Err = "ErrWrongLeader"
	ErrWrongGroup  Err = "ErrWrongGroup"
)

type TVersion uint64

type GetArgs struct {
	Key string
}

type GetReply struct {
	Value   string
	Version TVersion
	Err     Err
}

type PutArgs struct {
	Key     string
	Value   string
	Version TVersion
}

type PutReply struct {
	Err Err
}

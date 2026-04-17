package raft

type Transport interface {
	Call(method string, args interface{}, reply interface{}) bool
}

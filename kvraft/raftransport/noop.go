package raftransport

type Noop struct{}

func (Noop) Call(string, any, any) bool { return false }

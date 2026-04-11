package raftransport

// Noop is used for a peer's own index in the transport slice; Raft never
// sends RPCs to itself.
type Noop struct{}

func (Noop) Call(string, interface{}, interface{}) bool { return false }

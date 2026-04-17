package raftransport

// Noop occupies a node's own slot in the peer-transport slice so that every
// index is valid without nil checks at call sites.  Raft never sends RPCs to
// itself, so Call should never be invoked on Noop; returning false (i.e. "RPC
// failed") is the safest fallback in case it ever is called by mistake.
type Noop struct{}

func (Noop) Call(string, interface{}, interface{}) bool { return false }

package sm

type Op struct {
	Me  int
	Id  int64 // Unique ID to match Submit with the applied result
	Req any
}

type StateMachine interface {
	DoOp(any) any
	Snapshot() ([]byte, error)
	Restore([]byte) error
}

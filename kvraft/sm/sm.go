package sm

type Op struct {
	Me  int
	Id  int64
	Req any
}

type StateMachine interface {
	DoOp(any) any
	Snapshot() ([]byte, error)
	Restore([]byte) error
}

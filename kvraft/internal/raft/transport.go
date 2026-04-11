package raft

//type ClientEnd interface {
//	Call(serviceMethod string, args any, reply any) bool
//}
//
//rpc.Register(kvServer)
//listener, _ := net.Listen("tcp", ":8000")
//rpc.Accept(listener)
//

type Transport interface {
	//Call(server int, method string, args interface{}, reply interface{}) bool
	Call(method string, args interface{}, reply interface{}) bool
}

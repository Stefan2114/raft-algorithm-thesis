package testutils

import (
	"bytes"
	"encoding/gob"
	"log"
	"math/rand"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Network struct {
	mu           sync.Mutex
	servers      map[int]any
	connections  map[int]map[int]bool
	dropRate     float64
	delayRate    float64
	maxDelayMs   int
	isReliable   bool
	isLongDelays bool
	rpcCounts    map[int]*int64
}

func NewNetwork() *Network {
	return &Network{
		servers:     make(map[int]any),
		connections: make(map[int]map[int]bool),
		rpcCounts:   make(map[int]*int64),
		isReliable:  true,
	}
}

func (n *Network) SetReliability(reliable bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.isReliable = reliable
	if !reliable {
		n.dropRate = 0.1
		n.delayRate = 0.1
		n.maxDelayMs = 200
	}
}

func (n *Network) GetRPCCount(id int) int {
	n.mu.Lock()
	defer n.mu.Unlock()
	if count, ok := n.rpcCounts[id]; ok {
		return int(atomic.LoadInt64(count))
	}
	return 0
}

func (n *Network) ResetRPCCounts() {
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, count := range n.rpcCounts {
		atomic.StoreInt64(count, 0)
	}
}

func (n *Network) AddServer(id int, server any) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.servers[id] = server
	n.connections[id] = make(map[int]bool)
	n.rpcCounts[id] = new(int64)
	for otherId := range n.servers {
		if id != otherId {
			n.connections[id][otherId] = true
			n.connections[otherId][id] = true
		}
	}
}

func (n *Network) Connect(from int, to int) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.connections[from][to] = true
}

func (n *Network) Disconnect(from int, to int) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.connections[from][to] = false
}

func (n *Network) Call(from int, to int, method string, args any, reply any) bool {
	n.mu.Lock()
	server, exists := n.servers[to]
	connected := n.connections[from][to] && n.connections[to][from]
	drop := !n.isReliable && rand.Float64() < n.dropRate
	delay := !n.isReliable && rand.Float64() < n.delayRate
	maxDelay := n.maxDelayMs
	countPtr := n.rpcCounts[to]
	n.mu.Unlock()

	if !exists || !connected || drop {
		return false
	}

	if countPtr != nil {
		atomic.AddInt64(countPtr, 1)
	}

	if delay {
		time.Sleep(time.Duration(rand.Intn(maxDelay)) * time.Millisecond)
	}

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(args); err != nil {
		log.Printf("testutils: gob encode error %v", err)
		return false
	}

	argsType := reflect.TypeOf(args)
	if argsType.Kind() == reflect.Ptr {
		argsType = argsType.Elem()
	}
	newArgs := reflect.New(argsType).Interface()
	dec := gob.NewDecoder(&buf)
	if err := dec.Decode(newArgs); err != nil {
		log.Printf("testutils: gob decode error %v", err)
		return false
	}

	dot := strings.LastIndex(method, ".")
	methodName := method
	if dot >= 0 {
		methodName = method[dot+1:]
	}
	methodValue := reflect.ValueOf(server).MethodByName(methodName)
	if !methodValue.IsValid() {
		log.Printf("testutils: invalid method %v (resolved to %v)", method, methodName)
		return false
	}

	in := []reflect.Value{reflect.ValueOf(newArgs), reflect.ValueOf(reply)}
	methodValue.Call(in)

	return true
}

type MockTransport struct {
	net  *Network
	me   int
	peer int
}

func (t *MockTransport) Call(method string, args any, reply any) bool {
	return t.net.Call(t.me, t.peer, method, args, reply)
}

func MakeMockTransports(net *Network, me int, numServers int) []any {
	transports := make([]any, numServers)
	for i := 0; i < numServers; i++ {
		transports[i] = &MockTransport{
			net:  net,
			me:   me,
			peer: i,
		}
	}
	return transports
}

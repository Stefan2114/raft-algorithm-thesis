package kvserver

import (
	"bytes"
	"encoding/gob"
	"sync"

	"kvraft/api"
)

type Store struct {
	mu sync.Mutex
	kv map[string]struct {
		Value   string
		Version api.TVersion
	}
}

func NewStore() *Store {
	return &Store{
		kv: make(map[string]struct {
			Value   string
			Version api.TVersion
		}),
	}
}

func (s *Store) DoOp(req any) any {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch x := req.(type) {
	case api.GetArgs:
		cur, ok := s.kv[x.Key]
		if !ok {
			return api.GetReply{Err: api.ErrNoKey}
		}
		return api.GetReply{Value: cur.Value, Version: cur.Version, Err: api.OK}

	case api.PutArgs:
		cur, ok := s.kv[x.Key]
		if !ok {
			if x.Version != 0 {
				return api.PutReply{Err: api.ErrVersion}
			}
			s.kv[x.Key] = struct {
				Value   string
				Version api.TVersion
			}{Value: x.Value, Version: 1}
			return api.PutReply{Err: api.OK}
		}
		if cur.Version != x.Version {
			return api.PutReply{Err: api.ErrVersion}
		}
		s.kv[x.Key] = struct {
			Value   string
			Version api.TVersion
		}{Value: x.Value, Version: cur.Version + 1}
		return api.PutReply{Err: api.OK}

	default:
		return api.PutReply{Err: api.ErrWrongLeader}
	}
}

func (s *Store) Snapshot() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	w := new(bytes.Buffer)
	enc := gob.NewEncoder(w)
	if err := enc.Encode(s.kv); err != nil {
		panic(err)
	}
	return w.Bytes()
}

func (s *Store) Restore(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(data) == 0 {
		s.kv = make(map[string]struct {
			Value   string
			Version api.TVersion
		})
		return
	}
	dec := gob.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&s.kv); err != nil {
		panic(err)
	}
}

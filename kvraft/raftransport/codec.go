package raftransport

import (
	"bytes"
	"encoding/gob"
	"fmt"

	"kvraft/internal/raft"
	kvpb "kvraft/pb"
)

func entriesToProto(entries []raft.Entry) ([]*kvpb.LogEntry, error) {
	out := make([]*kvpb.LogEntry, len(entries))
	for i := range entries {
		var buf bytes.Buffer
		if err := gob.NewEncoder(&buf).Encode(&entries[i].Command); err != nil {
			return nil, err
		}
		out[i] = &kvpb.LogEntry{
			Index:   int32(entries[i].Index),
			Term:    int32(entries[i].Term),
			Command: buf.Bytes(),
		}
	}
	return out, nil
}

func entriesFromProto(entries []*kvpb.LogEntry) ([]raft.Entry, error) {
	out := make([]raft.Entry, len(entries))
	for i, e := range entries {
		dec := gob.NewDecoder(bytes.NewReader(e.Command))
		var cmd any
		if err := dec.Decode(&cmd); err != nil {
			return nil, fmt.Errorf("decode command: %w", err)
		}
		out[i] = raft.Entry{
			Index:   int(e.Index),
			Term:    int(e.Term),
			Command: cmd,
		}
	}
	return out, nil
}

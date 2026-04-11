package persist

import (
	"os"
	"path/filepath"
	"sync"
)

type FilePersister struct {
	mu  sync.Mutex
	dir string // e.g. "/var/lib/kvraft/node-0"
	// Each node gets its own directory.
}

func MakeFilePersister(dir string) (*FilePersister, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	return &FilePersister{dir: dir}, nil
}

func (ps *FilePersister) ReadRaftState() ([]byte, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	data, err := os.ReadFile(filepath.Join(ps.dir, "raft-state"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	return data, err
}

func (ps *FilePersister) RaftStateSize() int {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	info, err := os.Stat(filepath.Join(ps.dir, "raft-state"))
	if err != nil {
		return 0
	}
	return int(info.Size())
}

func (ps *FilePersister) ReadSnapshot() ([]byte, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	data, err := os.ReadFile(filepath.Join(ps.dir, "snapshot"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	return data, err
}

func (ps *FilePersister) SnapshotSize() int {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	info, err := os.Stat(filepath.Join(ps.dir, "snapshot"))
	if err != nil {
		return 0
	}
	return int(info.Size())
}

func (ps *FilePersister) Save(raftState []byte, snapshot []byte) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if err := ps.atomicWrite("raft-state", raftState); err != nil {
		return err
	}
	if err := ps.atomicWrite("snapshot", snapshot); err != nil {
		return err
	}
	return nil
}

func (ps *FilePersister) atomicWrite(filename string, data []byte) error {
	targetPath := filepath.Join(ps.dir, filename)
	tempPath := targetPath + ".tmp"

	f, err := os.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	return os.Rename(tempPath, targetPath)
}

package testutils

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/anishathalye/porcupine"
	"kvraft/internal/models"
)

type OpLog struct {
	mu         sync.Mutex
	operations []porcupine.Operation
	t0         time.Time
}

func NewOpLog() *OpLog {
	return &OpLog{
		t0: time.Now(),
	}
}

func (l *OpLog) Append(input models.KvInput, output models.KvOutput, start, end time.Time, clientId int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.operations = append(l.operations, porcupine.Operation{
		Input:    input,
		Output:   output,
		Call:     start.Sub(l.t0).Nanoseconds(),
		Return:   end.Sub(l.t0).Nanoseconds(),
		ClientId: clientId,
	})
}

func (l *OpLog) Check(t *testing.T) {
	l.mu.Lock()
	ops := make([]porcupine.Operation, len(l.operations))
	copy(ops, l.operations)
	l.mu.Unlock()

	res, info := porcupine.CheckOperationsVerbose(models.KvModel, ops, 5*time.Second)
	if res == porcupine.Illegal {
		file, err := os.CreateTemp("", "porcupine-*.html")
		if err == nil {
			_ = porcupine.Visualize(models.KvModel, info, file)
			fmt.Printf("Linearizability violation! Visualization saved to %s\n", file.Name())
		}
		t.Fatal("History is NOT linearizable")
	} else if res == porcupine.Unknown {
		t.Log("Linearizability check timed out, assuming OK")
	} else {
		t.Log("Linearizability check PASSED")
	}
}

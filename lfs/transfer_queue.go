package lfs

import (
	"sync"

	"github.com/github/git-lfs/vendor/_nuts/github.com/rubyist/tracerx"
)

const (
	batchSize = 100
)

type Transferable interface {
	Transfer(CopyCallback) error
	Object() *objectResource
	Oid() string
	Size() int64
	Name() string
	SetObject(*objectResource)
}

// TransferQueue provides a queue that will allow concurrent transfers.
type TransferQueue struct {
	meter         *ProgressMeter
	workers       int // Number of transfer workers to spawn
	transferKind  string
	errors        []error
	transferables map[string]Transferable
	transferc     chan Transferable // Channel for processing transfers
	errorc        chan error        // Channel for processing errors
	errorwait     sync.WaitGroup
	wait          sync.WaitGroup
}

// newTransferQueue builds a TransferQueue, allowing `workers` concurrent transfers.
func newTransferQueue(files int, size int64, dryRun bool) *TransferQueue {
	q := &TransferQueue{
		meter:         NewProgressMeter(files, size, dryRun),
		transferc:     make(chan Transferable, batchSize),
		errorc:        make(chan error),
		workers:       Config.ConcurrentTransfers(),
		transferables: make(map[string]Transferable),
	}

	q.errorwait.Add(1)

	q.run()

	return q
}

// Add adds a Transferable to the transfer queue.
func (q *TransferQueue) Add(transfer Transferable) {
	q.wait.Add(1)
	q.transferables[transfer.Oid()] = transfer
	
	q.meter.Add(transfer.Name())
	q.transferc <- transfer

}

// Wait waits for the queue to finish processing all transfers. Once Wait is
// called, Add will no longer add transferables to the queue.
func (q *TransferQueue) Wait() {
	q.wait.Wait()

	close(q.transferc)
	close(q.errorc)

	q.meter.Finish()
	q.errorwait.Wait()
}

// This goroutine collects errors returned from transfers
func (q *TransferQueue) errorCollector() {
	for err := range q.errorc {
		q.errors = append(q.errors, err)
	}
	q.errorwait.Done()
}

func (q *TransferQueue) transferWorker() {
	for transfer := range q.transferc {
		cb := func(total, read int64, current int) error {
			q.meter.TransferBytes(q.transferKind, transfer.Name(), read, total, current)
			return nil
		}

		if err := transfer.Transfer(cb); err != nil {
				q.errorc <- err
		}

		q.meter.FinishTransfer(transfer.Name())

		q.wait.Done()
	}
}

func (q *TransferQueue) run() {
	go q.errorCollector()

	tracerx.Printf("tq: starting %d transfer workers", q.workers)
	for i := 0; i < q.workers; i++ {
		go q.transferWorker()
	}
}

// Errors returns any errors encountered during transfer.
func (q *TransferQueue) Errors() []error {
	return q.errors
}

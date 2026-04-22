package tui

// BuildStatus is the classic TUI build lifecycle summary.
type BuildStatus struct {
	Running bool
	Done    bool
	Error   string
}

// WorkerStatus describes the current state of one extraction worker.
type WorkerStatus struct {
	ID     int
	File   string // current file being processed (empty = idle)
	Idle   bool
	Done   bool
	ErrMsg string
}

// WorkerUpdate is sent on the worker channel when a worker changes state.
type WorkerUpdate struct {
	Status WorkerStatus
}

// Pool manages N worker goroutines and sends status updates on a channel.
// Callers create a Pool, call Run(), and range over Updates() to display
// per-worker progress.
type Pool struct {
	n       int
	jobs    <-chan string // file paths
	results chan<- workerResult
	updates chan WorkerUpdate
}

type workerResult struct {
	file string
	err  error
}

// NewPool creates a Pool with n workers reading from jobs.
func NewPool(n int, jobs <-chan string, results chan<- workerResult) *Pool {
	return &Pool{
		n:       n,
		jobs:    jobs,
		results: results,
		updates: make(chan WorkerUpdate, n*4),
	}
}

// Updates returns the channel of WorkerUpdate messages.
func (p *Pool) Updates() <-chan WorkerUpdate {
	return p.updates
}

// Run launches n worker goroutines. Each goroutine reads from jobs,
// sends WorkerUpdates, and writes results. Run returns when all workers exit.
func (p *Pool) Run(process func(file string) error) {
	done := make(chan struct{})
	for i := 0; i < p.n; i++ {
		go func(id int) {
			defer func() { done <- struct{}{} }()
			for file := range p.jobs {
				p.updates <- WorkerUpdate{Status: WorkerStatus{ID: id, File: file, Idle: false}}
				err := process(file)
				if err != nil {
					p.updates <- WorkerUpdate{Status: WorkerStatus{ID: id, File: file, Idle: true, ErrMsg: err.Error()}}
				} else {
					p.updates <- WorkerUpdate{Status: WorkerStatus{ID: id, File: "", Idle: true}}
				}
			}
			p.updates <- WorkerUpdate{Status: WorkerStatus{ID: id, Idle: true, Done: true}}
		}(i)
	}
	for i := 0; i < p.n; i++ {
		<-done
	}
	close(p.updates)
}

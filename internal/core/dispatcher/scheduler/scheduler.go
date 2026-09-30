package scheduler

type Scheduler interface {
	// Submit submits a task to the scheduler
	Submit(task *DispatchTask) error
	// Start starts the scheduler
	Start() error
	// Stop stops the scheduler and waits for pending tasks if possible
	Stop() error
}

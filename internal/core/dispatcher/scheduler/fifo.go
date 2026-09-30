package scheduler

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
)

// ErrQueueFull indicates scheduler queue overflow.
var ErrQueueFull = errors.New("scheduler queue full")

// TaskExecutor is a function that executes a task
// It is injected into the Scheduler to decouple execution logic
type TaskExecutor func(task *DispatchTask) error

type FifoScheduler struct {
	queue            chan *DispatchTask
	wg               sync.WaitGroup
	stop             chan struct{}
	executor         TaskExecutor
	started          bool
	mu               sync.Mutex
	overflowStrategy string
	rejectedCount    int64
	droppedCount     int64
	blockedNanos     int64
	maxQueueDepth    int64
}

// NewFifoScheduler creates a new FIFO scheduler
func NewFifoScheduler(buffer int, overflowStrategy string, executor TaskExecutor) *FifoScheduler {
	if buffer <= 0 {
		buffer = 100
	}
	if overflowStrategy == "" {
		overflowStrategy = "reject"
	}
	return &FifoScheduler{
		queue:            make(chan *DispatchTask, buffer),
		stop:             make(chan struct{}),
		executor:         executor,
		overflowStrategy: overflowStrategy,
	}
}

func (s *FifoScheduler) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return errors.New("scheduler already started")
	}

	s.wg.Add(1)
	go s.loop()
	s.started = true
	return nil
}

func (s *FifoScheduler) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return nil
	}

	close(s.stop)
	s.wg.Wait()
	s.started = false
	return nil
}

func (s *FifoScheduler) Submit(task *DispatchTask) error {
	if task == nil {
		return errors.New("dispatch task is nil")
	}
	// The queue owns its task; callers may safely reuse their values after Submit.
	task = task.Clone()

	depth := int64(len(s.queue))
	for {
		maxSeen := atomic.LoadInt64(&s.maxQueueDepth)
		if depth <= maxSeen || atomic.CompareAndSwapInt64(&s.maxQueueDepth, maxSeen, depth) {
			break
		}
	}

	switch s.overflowStrategy {
	case "block":
		start := time.Now()
		select {
		case s.queue <- task:
			atomic.AddInt64(&s.blockedNanos, time.Since(start).Nanoseconds())
			return nil
		case <-s.stop:
			return errors.New("scheduler stopped")
		}
	case "drop_oldest":
		select {
		case s.queue <- task:
			return nil
		default:
			select {
			case <-s.queue:
				atomic.AddInt64(&s.droppedCount, 1)
				pkglog.Warn("Scheduler queue overflow: dropped oldest task (dropped=%d, queue_len=%d)",
					atomic.LoadInt64(&s.droppedCount), len(s.queue))
			default:
			}
			select {
			case s.queue <- task:
				return nil
			default:
				atomic.AddInt64(&s.rejectedCount, 1)
				return ErrQueueFull
			}
		}
	default: // reject
		select {
		case s.queue <- task:
			return nil
		default:
			atomic.AddInt64(&s.rejectedCount, 1)
			return ErrQueueFull
		}
	}
}

func (s *FifoScheduler) loop() {
	defer s.wg.Done()
	for {
		select {
		case task := <-s.queue:
			if err := s.execute(task); err != nil {
				pkglog.Error("Scheduler failed to execute task (Rule: %s, To: %s): %v", task.RuleID, task.ToNode, err)
			}
		case <-s.stop:
			// Optional: drain queue
			return
		}
	}
}

func (s *FifoScheduler) execute(task *DispatchTask) error {
	if s.executor != nil {
		return s.executor(task)
	}
	// Warning: No executor configured
	pkglog.Warn("No executor configured for scheduler")
	return nil
}

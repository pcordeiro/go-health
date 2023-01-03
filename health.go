package health

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"
)

type (
	Status string

	// Component descriptive values about the component for which checks are made
	Component struct {
		// Name is the name of the component.
		Name string `json:"name"`
		// Version is the component version.
		Version string `json:"version"`
	}

	// System runtime variables about the go process.
	System struct {
		// Version is the go version.
		Version string `json:"version"`
		// GoroutinesCount is the number of the current goroutines.
		GoroutinesCount int `json:"goroutines_count"`
		// TotalAllocBytes is the total bytes allocated.
		TotalAllocBytes int `json:"total_alloc_bytes"`
		// HeapObjectsCount is the number of objects in the go heap.
		HeapObjectsCount int `json:"heap_objects_count"`
		// TotalAllocBytes is the bytes allocated and not yet freed.
		AllocBytes int `json:"alloc_bytes"`
	}

	// CheckFunc is the func which executes the check.
	CheckFunc func(context.Context) error

	Check struct {
		Name      string
		Timeout   time.Duration
		SkipOnErr bool
		Check     CheckFunc
	}

	Result struct {
		// Status is the check status.
		Status Status `json:"status"`
		// Timestamp is the time in which the check occurred.
		Timestamp time.Time `json:"timestamp"`
		// Failures holds the failed checks along with their messages.
		Failures map[string]string `json:"failures,omitempty"`
		// System holds information of the go process.
		*System `json:"system,omitempty"`
		// Component holds information on the component for which checks are made
		Component `json:"component"`
	}

	Health struct {
		mu            sync.Mutex
		checks        map[string]Check
		maxConcurrent int
		systemInfo    bool
		component     Component
	}
)

const (
	StatusOK                 Status = "OK"
	StatusPartiallyAvailable Status = "Partially Available"
	StatusUnavailable        Status = "Unavailable"
	StatusTimeout            Status = "Timeout during health check"
)

func NewHealth(opts ...Option) (*Health, error) {
	h := &Health{
		checks:        make(map[string]Check),
		maxConcurrent: runtime.NumCPU(),
		systemInfo:    true,
	}

	for _, o := range opts {
		err := o(h)
		if err != nil {
			return nil, err
		}
	}

	return h, nil
}

// Register registers a check config to be performed.
func (h *Health) Register(c Check) error {
	if c.Timeout == 0 {
		c.Timeout = time.Second * 2
	}

	if c.Name == "" {
		return errors.New("health check must have a name to be registered")
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.checks[c.Name]; ok {
		return fmt.Errorf("health check %q is already registered", c.Name)
	}

	h.checks[c.Name] = c

	return nil
}

func (h *Health) Check(ctx context.Context) Result {
	h.mu.Lock()
	defer h.mu.Unlock()

	status := StatusOK
	failures := make(map[string]string)

	limiterCh := make(chan bool, h.maxConcurrent)
	defer close(limiterCh)

	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)

	for _, c := range h.checks {
		limiterCh <- true
		wg.Add(1)

		go func(c Check) {
			defer func() {
				<-limiterCh
				wg.Done()
			}()

			resCh := make(chan error)

			go func() {
				resCh <- c.Check(ctx)
				defer close(resCh)
			}()

			select {
			case <-time.After(c.Timeout):
				mu.Lock()
				defer mu.Unlock()

				failures[c.Name] = "Timeout"
				status = getAvailability(status, c.SkipOnErr)
			case res := <-resCh:
				mu.Lock()
				defer mu.Unlock()

				if res != nil {
					failures[c.Name] = res.Error()
					status = getAvailability(status, c.SkipOnErr)
				}
			}
		}(c)
	}

	wg.Wait()

	var systemMetrics *System
	if h.systemInfo {
		systemMetrics = newSystemMetrics()
	}

	return Result{
		Status:    status,
		Failures:  failures,
		System:    systemMetrics,
		Component: h.component,
		Timestamp: time.Now(),
	}
}

func newSystemMetrics() *System {
	s := runtime.MemStats{}
	runtime.ReadMemStats(&s)

	return &System{
		Version:          runtime.Version(),
		GoroutinesCount:  runtime.NumGoroutine(),
		TotalAllocBytes:  int(s.TotalAlloc),
		HeapObjectsCount: int(s.HeapObjects),
		AllocBytes:       int(s.Alloc),
	}
}

func getAvailability(s Status, skipOnErr bool) Status {
	if skipOnErr && s != StatusUnavailable {
		return StatusPartiallyAvailable
	}

	return StatusUnavailable
}

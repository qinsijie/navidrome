package musicengine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/navidrome/navidrome/log"
)

// Worker represents a single Goja VM instance prepared with user script runtime
type Worker struct {
	vm      *goja.Runtime
	program *goja.Program
}

// NewWorker initializes a Goja runtime, injects polyfills, and runs the precompiled program
func NewWorker(program *goja.Program) (*Worker, error) {
	vm := goja.New()
	if err := InjectPolyfills(vm); err != nil {
		return nil, fmt.Errorf("injecting polyfills: %w", err)
	}

	if program != nil {
		if _, err := vm.RunProgram(program); err != nil {
			return nil, fmt.Errorf("running program in worker: %w", err)
		}
	}

	return &Worker{
		vm:      vm,
		program: program,
	}, nil
}

// WorkerPool manages a pool of Worker instances for concurrent script execution
type WorkerPool struct {
	program *goja.Program
	pool    sync.Pool
}

// NewWorkerPool creates a new pool with the given precompiled program
func NewWorkerPool(program *goja.Program) *WorkerPool {
	wp := &WorkerPool{
		program: program,
	}
	wp.pool.New = func() any {
		w, err := NewWorker(wp.program)
		if err != nil {
			log.Error("MusicEngine: Failed to create worker in pool", err)
			return nil
		}
		return w
	}
	return wp
}

// Acquire gets a worker from the pool
func (wp *WorkerPool) Acquire() (*Worker, error) {
	val := wp.pool.Get()
	if val == nil {
		return nil, fmt.Errorf("failed to acquire worker from pool")
	}
	return val.(*Worker), nil
}

// Release returns a worker to the pool
func (wp *WorkerPool) Release(w *Worker) {
	if w != nil {
		wp.pool.Put(w)
	}
}

// ExecuteWithTimeout runs an action inside the worker within a deadline
func (wp *WorkerPool) ExecuteWithTimeout(ctx context.Context, timeout time.Duration, fn func(vm *goja.Runtime) (any, error)) (any, error) {
	w, err := wp.Acquire()
	if err != nil {
		return nil, err
	}
	defer wp.Release(w)

	timer := time.AfterFunc(timeout, func() {
		w.vm.Interrupt("execution timeout exceeded")
	})
	defer timer.Stop()

	return fn(w.vm)
}

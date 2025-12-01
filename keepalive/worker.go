// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package keepalive

import (
	"context"
	"sync"
	"sync/atomic"
)

// WorkerPool manages a pool of concurrent workers
type WorkerPool struct {
	size       int
	taskQueue  chan Task
	results    chan TaskResult
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
	running    int32
	completed  int32
	failed     int32
}

// Task represents a unit of work
type Task interface {
	Execute(ctx context.Context) error
	GetID() string
}

// TaskResult contains the result of a task execution
type TaskResult struct {
	TaskID  string
	Success bool
	Error   error
}

// FetchTask represents a block fetch task
type FetchTask struct {
	Block   *Block
	Fetcher *Fetcher
}

// GetID returns the task ID
func (t *FetchTask) GetID() string {
	return t.Block.URI
}

// Execute performs the fetch
func (t *FetchTask) Execute(ctx context.Context) error {
	result := t.Fetcher.FetchBlock(ctx, t.Block)
	if !result.Success {
		return &TaskError{Message: result.Error}
	}
	return nil
}

// InsertTask represents a block insert task
type InsertTask struct {
	Block    *Block
	Inserter *Inserter
}

// GetID returns the task ID
func (t *InsertTask) GetID() string {
	return t.Block.URI
}

// Execute performs the insert
func (t *InsertTask) Execute(ctx context.Context) error {
	result := t.Inserter.InsertBlock(ctx, t.Block)
	if !result.Success {
		return &TaskError{Message: result.Error}
	}
	return nil
}

// TaskError represents a task execution error
type TaskError struct {
	Message string
}

func (e *TaskError) Error() string {
	return e.Message
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(size int) *WorkerPool {
	ctx, cancel := context.WithCancel(context.Background())
	return &WorkerPool{
		size:      size,
		taskQueue: make(chan Task, size*2),
		results:   make(chan TaskResult, size*2),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Start starts the worker pool
func (p *WorkerPool) Start() {
	for i := 0; i < p.size; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
}

// Stop stops the worker pool
func (p *WorkerPool) Stop() {
	p.cancel()
	close(p.taskQueue)
	p.wg.Wait()
	close(p.results)
}

// Submit submits a task to the pool
func (p *WorkerPool) Submit(task Task) {
	select {
	case <-p.ctx.Done():
		return
	case p.taskQueue <- task:
		atomic.AddInt32(&p.running, 1)
	}
}

// Results returns the results channel
func (p *WorkerPool) Results() <-chan TaskResult {
	return p.results
}

// worker is a single worker goroutine
func (p *WorkerPool) worker(id int) {
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case task, ok := <-p.taskQueue:
			if !ok {
				return
			}

			err := task.Execute(p.ctx)
			result := TaskResult{
				TaskID:  task.GetID(),
				Success: err == nil,
				Error:   err,
			}

			atomic.AddInt32(&p.running, -1)
			if err == nil {
				atomic.AddInt32(&p.completed, 1)
			} else {
				atomic.AddInt32(&p.failed, 1)
			}

			select {
			case p.results <- result:
			case <-p.ctx.Done():
				return
			}
		}
	}
}

// Stats returns pool statistics
func (p *WorkerPool) Stats() (running, completed, failed int32) {
	return atomic.LoadInt32(&p.running),
		atomic.LoadInt32(&p.completed),
		atomic.LoadInt32(&p.failed)
}

// Pending returns the number of pending tasks
func (p *WorkerPool) Pending() int {
	return len(p.taskQueue)
}

// BatchProcessor processes tasks in batches
type BatchProcessor struct {
	pool     *WorkerPool
	batchSize int
}

// NewBatchProcessor creates a new batch processor
func NewBatchProcessor(workers, batchSize int) *BatchProcessor {
	return &BatchProcessor{
		pool:     NewWorkerPool(workers),
		batchSize: batchSize,
	}
}

// ProcessTasks processes a slice of tasks with progress updates
func (bp *BatchProcessor) ProcessTasks(ctx context.Context, tasks []Task, onProgress func(completed, total int)) error {
	bp.pool.Start()
	defer bp.pool.Stop()

	total := len(tasks)
	completed := 0

	// Start result collector
	done := make(chan struct{})
	var collectErr error

	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				collectErr = ctx.Err()
				return
			case result, ok := <-bp.pool.Results():
				if !ok {
					return
				}
				completed++
				if onProgress != nil {
					onProgress(completed, total)
				}
				if !result.Success && collectErr == nil {
					// Store first error but continue processing
					collectErr = result.Error
				}
				if completed >= total {
					return
				}
			}
		}
	}()

	// Submit tasks
	for _, task := range tasks {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			bp.pool.Submit(task)
		}
	}

	// Wait for completion
	<-done

	return collectErr
}

// Limiter provides rate limiting
type Limiter struct {
	tokens chan struct{}
}

// NewLimiter creates a new limiter with the given concurrency limit
func NewLimiter(limit int) *Limiter {
	return &Limiter{
		tokens: make(chan struct{}, limit),
	}
}

// Acquire acquires a token (blocks if at limit)
func (l *Limiter) Acquire(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case l.tokens <- struct{}{}:
		return nil
	}
}

// Release releases a token
func (l *Limiter) Release() {
	<-l.tokens
}

// TryAcquire tries to acquire a token without blocking
func (l *Limiter) TryAcquire() bool {
	select {
	case l.tokens <- struct{}{}:
		return true
	default:
		return false
	}
}

// Available returns the number of available tokens
func (l *Limiter) Available() int {
	return cap(l.tokens) - len(l.tokens)
}

package worker

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

var (
	ErrQueueFull = errors.New("worker queue is full")
	ErrNotFound  = errors.New("worker job not found")
	ErrInvalid   = errors.New("invalid worker request")
)

type Config struct {
	QueueSize, Concurrency     int
	ScriptRoots, WorktreeRoots []string
	Scripts                    map[string]string
}
type Request struct{ OrganizationID, Wave, Branch, Brief, Worktree, Script string }
type Job struct {
	ID      string
	Request Request
	State   string
	Err     error
	cancel  context.CancelFunc
}
type Runner func(context.Context, Request) error

type Executor struct {
	cfg   Config
	run   Runner
	mu    sync.Mutex
	next  uint64
	jobs  map[string]*Job
	queue chan *Job
	stop  chan struct{}
	once  sync.Once
}

func New(cfg Config) (*Executor, error) {
	if cfg.QueueSize < 1 {
		cfg.QueueSize = 16
	}
	if cfg.Concurrency < 1 {
		cfg.Concurrency = 1
	}
	e := &Executor{cfg: cfg, jobs: map[string]*Job{}, queue: make(chan *Job, cfg.QueueSize), stop: make(chan struct{})}
	e.run = e.runProcess
	for i := 0; i < cfg.Concurrency; i++ {
		go e.loop()
	}
	return e, nil
}
func (e *Executor) loop() {
	for {
		select {
		case j := <-e.queue:
			e.execute(j)
		case <-e.stop:
			return
		}
	}
}
func (e *Executor) execute(j *Job) {
	ctx, cancel := context.WithCancel(context.Background())
	e.mu.Lock()
	if j.State == "cancelled" {
		e.mu.Unlock()
		cancel()
		return
	}
	j.cancel = cancel
	j.State = "running"
	e.mu.Unlock()
	err := e.run(ctx, j.Request)
	cancel()
	e.mu.Lock()
	defer e.mu.Unlock()
	j.Err = err
	if j.State == "cancelled" || errors.Is(err, context.Canceled) {
		j.State = "cancelled"
	} else if err != nil {
		j.State = "failed"
	} else {
		j.State = "succeeded"
	}
	j.cancel = nil
}
func (e *Executor) Enqueue(req Request) (*Job, error) {
	if err := e.validate(req); err != nil {
		return nil, err
	}
	e.mu.Lock()
	e.next++
	j := &Job{ID: fmt.Sprintf("worker-%d", e.next), Request: req, State: "queued"}
	e.jobs[j.ID] = j
	e.mu.Unlock()
	select {
	case e.queue <- j:
		return j, nil
	default:
		e.mu.Lock()
		delete(e.jobs, j.ID)
		e.mu.Unlock()
		return nil, ErrQueueFull
	}
}
func (e *Executor) Get(id string) (*Job, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	j, ok := e.jobs[id]
	if !ok {
		return nil, ErrNotFound
	}
	copy := *j
	return &copy, nil
}
func (e *Executor) GetForOrganization(id, organizationID string) (*Job, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	j, ok := e.jobs[id]
	if !ok || j.Request.OrganizationID != organizationID {
		return nil, ErrNotFound
	}
	copy := *j
	return &copy, nil
}
func (e *Executor) Stop(id string) error {
	return e.stopJob(id, "")
}
func (e *Executor) StopForOrganization(id, organizationID string) error {
	return e.stopJob(id, organizationID)
}
func (e *Executor) stopJob(id, organizationID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	j, ok := e.jobs[id]
	if !ok || (organizationID != "" && j.Request.OrganizationID != organizationID) {
		return ErrNotFound
	}
	if j.cancel != nil {
		j.cancel()
		j.State = "cancelled"
		return nil
	}
	if j.State == "queued" {
		j.State = "cancelled"
		return nil
	}
	return nil
}
func (e *Executor) Close() { e.once.Do(func() { close(e.stop) }) }
func (e *Executor) validate(r Request) error {
	if strings.TrimSpace(r.OrganizationID) == "" || !validPart(r.Wave) || !validPart(r.Branch) || strings.TrimSpace(r.Brief) == "" {
		return fmt.Errorf("%w: missing or malformed worker arguments", ErrInvalid)
	}
	script, ok := e.cfg.Scripts[r.Script]
	if !ok || !insideAny(script, e.cfg.ScriptRoots) {
		return fmt.Errorf("%w: script is not allowlisted", ErrInvalid)
	}
	if !insideAny(r.Worktree, e.cfg.WorktreeRoots) {
		return fmt.Errorf("%w: worktree is outside permitted roots", ErrInvalid)
	}
	return nil
}
func validPart(s string) bool {
	return s != "" && filepath.Base(s) == s && !strings.ContainsAny(s, "\\/$;&|`\n\r\t")
}
func insideAny(path string, roots []string) bool {
	p, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	for _, root := range roots {
		r, err := filepath.Abs(root)
		if err == nil {
			rel, err := filepath.Rel(r, p)
			if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
				return true
			}
		}
	}
	return false
}
func (e *Executor) runProcess(ctx context.Context, r Request) error {
	script := e.cfg.Scripts[r.Script]
	cmd := exec.CommandContext(ctx, script, r.Wave, r.Branch, r.Brief, r.Worktree)
	cmd.Dir = r.Worktree
	return cmd.Run()
}

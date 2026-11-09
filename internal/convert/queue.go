package convert

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/rightconvert/rightconvert/internal/rules"
)

// State is where a job is.
type State int

const (
	Queued State = iota
	Running
	Done
	Failed
	Canceled
)

func (s State) String() string {
	return [...]string{"Queued", "Converting", "Done", "Failed", "Canceled"}[s]
}

// Job is one file to convert. Its fields are written by the queue; read
// them through Queue.Snapshot.
type Job struct {
	ID       int
	Input    string
	Output   string
	Rule     *rules.Rule
	State    State
	Progress float64 // 0..1, or -1 while the length is unknown
	Err      error

	cancel context.CancelFunc
}

// Runner converts one file; FFmpeg.Run is one.
type Runner func(ctx context.Context, r *rules.Rule, input, output string, progress func(float64)) error

// Queue runs jobs, at most Workers at a time.
type Queue struct {
	changed func()
	mu      sync.Mutex
	// run, resolveExt and sem are read when a job starts, so that setting
	// them affects the jobs that have not started.
	run        Runner
	resolveExt func(ctx context.Context, input string) (string, error)
	jobs       []*Job
	reserved   map[string]bool // outputs of queued and running jobs
	sem        chan struct{}
	nextID     int
	wg         sync.WaitGroup
}

// NewQueue returns a queue running run with the given number of workers,
// calling changed (from any goroutine) whenever a job changes.
func NewQueue(run Runner, workers int, changed func()) *Queue {
	if workers < 1 {
		workers = 1
	}
	if changed == nil {
		changed = func() {}
	}
	return &Queue{run: run, changed: changed, reserved: map[string]bool{}, sem: make(chan struct{}, workers)}
}

// SetRunner replaces how jobs convert, as when ffmpeg was located again;
// jobs already running keep theirs.
func (q *Queue) SetRunner(run Runner) {
	q.mu.Lock()
	q.run = run
	q.mu.Unlock()
}

// SetResolveExt sets how the output extension of rules whose To is "auto"
// is picked; FFmpeg.AudioExt is one.
func (q *Queue) SetResolveExt(fn func(ctx context.Context, input string) (string, error)) {
	q.mu.Lock()
	q.resolveExt = fn
	q.mu.Unlock()
}

// SetWorkers changes how many jobs run at once, for the jobs that have not
// started.
func (q *Queue) SetWorkers(n int) {
	q.mu.Lock()
	q.sem = make(chan struct{}, max(1, n))
	q.mu.Unlock()
}

// Add queues input for conversion by r. It fails when the rule does not
// take the file's extension.
func (q *Queue) Add(r *rules.Rule, input string) (*Job, error) {
	abs, err := filepath.Abs(input)
	if err != nil {
		return nil, err
	}
	if fi, err := os.Stat(abs); err != nil {
		return nil, err
	} else if fi.IsDir() {
		return nil, fmt.Errorf("%s is a folder", abs)
	}
	if !r.Accepts(filepath.Ext(abs)) {
		return nil, fmt.Errorf("%q does not convert %s files", r.Label, filepath.Ext(abs))
	}
	q.mu.Lock()
	q.nextID++
	j := &Job{ID: q.nextID, Input: abs, Rule: r, State: Queued}
	if !r.AutoTo() {
		// Otherwise the worker names it, once it knows the extension.
		j.Output = q.reserve(abs, r.Suffix, r.To)
	}
	ctx, cancel := context.WithCancel(context.Background())
	j.cancel = cancel
	q.jobs = append(q.jobs, j)
	q.wg.Add(1)
	q.mu.Unlock()
	q.changed()
	go q.work(ctx, j)
	return j, nil
}

// reserve picks "name{suffix}.ext" beside input, or "name{suffix} (n).ext"
// when that exists or another job writes it, and keeps it from other jobs.
// q.mu is held.
func (q *Queue) reserve(input, suffix, ext string) string {
	p := q.uniqueOutput(input, suffix, ext)
	q.reserved[strings.ToLower(p)] = true
	return p
}

func (q *Queue) uniqueOutput(input, suffix, ext string) string {
	dir := filepath.Dir(input)
	base := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input)) + suffix
	for n := 1; ; n++ {
		name := base
		if n > 1 {
			name = fmt.Sprintf("%s (%d)", base, n)
		}
		p := filepath.Join(dir, name+"."+ext)
		if q.reserved[strings.ToLower(p)] {
			continue
		}
		if _, err := os.Lstat(p); errors.Is(err, os.ErrNotExist) {
			return p
		}
	}
}

func (q *Queue) work(ctx context.Context, j *Job) {
	defer q.wg.Done()
	defer func() {
		q.mu.Lock()
		delete(q.reserved, strings.ToLower(j.Output))
		q.mu.Unlock()
		q.changed()
	}()
	q.mu.Lock()
	sem := q.sem
	q.mu.Unlock()
	select {
	case sem <- struct{}{}:
	case <-ctx.Done():
		q.set(j, func() { j.State = Canceled })
		return
	}
	defer func() { <-sem }()
	q.mu.Lock()
	run, resolve := q.run, q.resolveExt
	q.mu.Unlock()
	if j.Output == "" {
		if resolve == nil {
			resolve = func(context.Context, string) (string, error) {
				return "", errors.New(`no way to tell the output format of to = "auto"`)
			}
		}
		ext, err := resolve(ctx, j.Input)
		if err != nil {
			q.set(j, func() {
				if ctx.Err() != nil {
					j.State = Canceled
				} else {
					j.State, j.Err = Failed, err
				}
			})
			return
		}
		q.set(j, func() { j.Output = q.reserve(j.Input, j.Rule.Suffix, ext) })
	}
	q.set(j, func() { j.State, j.Progress = Running, 0 })
	err := run(ctx, j.Rule, j.Input, j.Output, func(p float64) {
		q.set(j, func() { j.Progress = p })
	})
	q.set(j, func() {
		switch {
		case ctx.Err() != nil:
			j.State = Canceled
		case err != nil:
			j.State, j.Err = Failed, err
		default:
			j.State, j.Progress = Done, 1
		}
	})
}

func (q *Queue) set(j *Job, fn func()) {
	q.mu.Lock()
	fn()
	q.mu.Unlock()
	q.changed()
}

// Cancel stops a queued or running job.
func (q *Queue) Cancel(id int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, j := range q.jobs {
		if j.ID == id {
			j.cancel()
		}
	}
}

// CancelAll stops every job.
func (q *Queue) CancelAll() {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, j := range q.jobs {
		j.cancel()
	}
}

// ClearFinished drops the jobs that are no longer queued or running.
func (q *Queue) ClearFinished() {
	q.mu.Lock()
	keep := q.jobs[:0]
	for _, j := range q.jobs {
		if j.State == Queued || j.State == Running {
			keep = append(keep, j)
		}
	}
	clear(q.jobs[len(keep):])
	q.jobs = keep
	q.mu.Unlock()
	q.changed()
}

// Snapshot returns copies of the jobs, in the order they were added.
func (q *Queue) Snapshot() []Job {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]Job, len(q.jobs))
	for i, j := range q.jobs {
		out[i] = *j
		out[i].cancel = nil
	}
	return out
}

// Active reports whether any job is queued or running.
func (q *Queue) Active() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, j := range q.jobs {
		if j.State == Queued || j.State == Running {
			return true
		}
	}
	return false
}

// Wait blocks until every job added so far has finished.
func (q *Queue) Wait() { q.wg.Wait() }

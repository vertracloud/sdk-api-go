package vertracloud

import (
	"context"
	"fmt"
	"time"
)

// DefaultWaitTimeout is the total wait of the snapshot job and stack waits
// (SnapshotJobs.Wait, Snapshots.CreateAndWait, Apps.Download, the stack
// WaitFor* and *AndWait) when WaitOptions.Timeout is zero: 30 minutes.
const DefaultWaitTimeout = 30 * time.Minute

// WaitTimeoutError is returned by a wait helper when WaitOptions.Timeout (or
// DefaultWaitTimeout) passes before the work ends. The work itself keeps going
// on the server. errors.Is(err, context.DeadlineExceeded) is true.
type WaitTimeoutError struct {
	// Timeout is the total wait that passed.
	Timeout time.Duration
	// Job is the last state read of a snapshot job, with its Progress while it
	// was running. Nil for the other waits.
	Job *SnapshotJob
}

func (e *WaitTimeoutError) Error() string {
	switch {
	case e.Job != nil:
		return fmt.Sprintf("WAIT_TIMEOUT: snapshot job %s still %s after %s", e.Job.ID, e.Job.Status, e.Timeout)
	}
	return fmt.Sprintf("WAIT_TIMEOUT: still waiting after %s", e.Timeout)
}

// Is makes errors.Is(err, context.DeadlineExceeded) true.
func (e *WaitTimeoutError) Is(target error) bool { return target == context.DeadlineExceeded }

// poller paces a wait: the interval between polls and the total deadline.
type poller struct {
	interval time.Duration
	timeout  time.Duration // zero: no limit besides ctx
	deadline time.Time
}

// newPoller reads opts (which may be nil). defTimeout applies when opts.Timeout
// is zero; a negative opts.Timeout means no limit besides ctx.
func newPoller(opts *WaitOptions, defInterval, defTimeout time.Duration) poller {
	p := poller{interval: defInterval, timeout: defTimeout}
	if opts != nil {
		if opts.Interval > 0 {
			p.interval = opts.Interval
		}
		if opts.Timeout != 0 {
			p.timeout = opts.Timeout
		}
	}
	if p.timeout < 0 {
		p.timeout = 0
	}
	if p.timeout > 0 {
		p.deadline = time.Now().Add(p.timeout)
	}
	return p
}

// next sleeps one interval. It reports false, with a nil error, when the next
// poll would fall after the deadline, and ctx.Err() when ctx ends first.
func (p poller) next(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if !p.deadline.IsZero() && time.Now().Add(p.interval).After(p.deadline) {
		return false, nil
	}
	t := time.NewTimer(p.interval)
	select {
	case <-ctx.Done():
		t.Stop()
		return false, ctx.Err()
	case <-t.C:
		return true, nil
	}
}

// remaining is what is left of the deadline for a wait that shares it (the
// jobs of a stack): -1 means no limit; a spent deadline gives a tiny positive
// value so the next wait times out at once.
func (p poller) remaining() time.Duration {
	if p.deadline.IsZero() {
		return -1
	}
	if left := time.Until(p.deadline); left > 0 {
		return left
	}
	return time.Nanosecond
}

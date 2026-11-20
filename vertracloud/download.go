package vertracloud

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/vertracloud/sdk-api-go/internal/reqopts"
	"github.com/vertracloud/sdk-api-go/rest"
)

// DefaultDownloadIdleTimeout is how long a DownloadToFile may go without a
// single byte before it fails with ErrDownloadStalled. The clock restarts at
// every read; there is no total deadline.
const DefaultDownloadIdleTimeout = 60 * time.Second

// ErrDownloadStalled is returned (wrapped) by the DownloadToFile methods when
// no byte arrived for the idle timeout (DefaultDownloadIdleTimeout, or the
// value of rest.WithIdleTimeout).
var ErrDownloadStalled = errors.New("vertracloud: no data received; the download stalled")

// downloadToFile copies the body that open returns into path through a
// temporary file next to it, renamed to path only when the body ends. Memory
// is constant (one buffer), whatever the size. Any failure, stall or
// cancellation of ctx removes the temporary file, so path never holds a
// partial file. The file is created with mode 0600 (a snapshot can hold
// secrets).
func downloadToFile(ctx context.Context, open func(ctx context.Context) (io.ReadCloser, error), path string, opts []rest.RequestOpt) (int64, error) {
	var cfg reqopts.Config
	for _, opt := range opts {
		opt(&cfg)
	}
	idle := cfg.IdleTimeout
	if idle == 0 {
		idle = DefaultDownloadIdleTimeout
	}

	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	body, err := open(ctx)
	if err != nil {
		return 0, err
	}
	defer body.Close()

	var suffix [6]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return 0, fmt.Errorf("vertracloud: temporary file name: %w", err)
	}
	temp := path + "." + hex.EncodeToString(suffix[:]) + ".part"
	f, err := os.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return 0, err
	}

	var timer *time.Timer
	if idle > 0 {
		timer = time.AfterFunc(idle, func() { cancel(ErrDownloadStalled) })
		defer timer.Stop()
	}
	n, copyErr := io.CopyBuffer(f, &idleReader{r: body, timer: timer, idle: idle}, make([]byte, 1<<20))
	if timer != nil {
		timer.Stop()
	}
	closeErr := f.Close()
	if copyErr == nil {
		copyErr = closeErr
	}
	if copyErr == nil {
		copyErr = ctx.Err()
		if errors.Is(context.Cause(ctx), ErrDownloadStalled) {
			copyErr = context.Cause(ctx)
		}
	}
	if copyErr == nil {
		copyErr = os.Rename(temp, path)
	}
	if copyErr != nil {
		_ = os.Remove(temp)
		if errors.Is(context.Cause(ctx), ErrDownloadStalled) {
			return n, fmt.Errorf("%w (%s without a byte)", ErrDownloadStalled, idle)
		}
		return n, copyErr
	}
	return n, nil
}

// idleReader restarts the stall timer after every read that returned bytes.
type idleReader struct {
	r     io.Reader
	timer *time.Timer
	idle  time.Duration
}

func (r *idleReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if n > 0 && r.timer != nil {
		r.timer.Reset(r.idle)
	}
	return n, err
}

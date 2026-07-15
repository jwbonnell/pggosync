package sync

import (
	"bytes"
	"io"
	"sync"
)

// SafeBuffer is a concurrent-safe, bounded streaming buffer. A writer goroutine calls Write and
// then SetDone (or SetDoneWithError) when finished. A reader goroutine calls Read and receives
// io.EOF once all data has been consumed and SetDone was called. If the writer encountered an
// error, Read returns that error instead.
//
// When maxBytes > 0, Write blocks once the buffered (unread) length reaches maxBytes and resumes
// as the reader drains it — this is the backpressure that bounds prefetch memory. Close unblocks a
// writer parked at the cap (returning io.ErrClosedPipe) so a torn-down sync doesn't leak the
// prefetch goroutine.
type SafeBuffer struct {
	mu       sync.Mutex
	cond     *sync.Cond
	buf      bytes.Buffer
	maxBytes int // <= 0 means unbounded
	done     bool
	doneErr  error
	closed   bool
}

// NewSafeBuffer returns a SafeBuffer capped at maxBytes unread bytes (<= 0 for unbounded).
func NewSafeBuffer(maxBytes int) *SafeBuffer {
	sb := &SafeBuffer{maxBytes: maxBytes}
	sb.cond = sync.NewCond(&sb.mu)
	return sb
}

// Write appends data to the underlying buffer under the lock and signals any blocked reader. When
// the buffer is bounded, it first blocks until the unread length drops below the cap (or the buffer
// is closed, in which case it returns io.ErrClosedPipe). The cap is soft: a single write is never
// split, so the buffer may overshoot by one chunk.
func (b *SafeBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for b.maxBytes > 0 && b.buf.Len() >= b.maxBytes && !b.closed {
		b.cond.Wait()
	}
	if b.closed {
		return 0, io.ErrClosedPipe
	}
	n, err = b.buf.Write(p)
	b.cond.Broadcast()
	return
}

// Read returns available data; blocks when the buffer is empty but the writer has not yet called
// SetDone. After draining data it broadcasts so a writer blocked on the cap can resume.
func (b *SafeBuffer) Read(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for {
		n, err = b.buf.Read(p)
		if err != io.EOF {
			b.cond.Broadcast()
			return n, err
		}
		if b.done {
			if b.doneErr != nil {
				return 0, b.doneErr
			}
			return 0, io.EOF
		}
		if b.closed {
			return 0, io.ErrClosedPipe
		}
		b.cond.Wait()
	}
}

// SetDone signals that the writer finished successfully; the next Read after the buffer drains returns io.EOF.
func (b *SafeBuffer) SetDone() {
	b.mu.Lock()
	b.done = true
	b.mu.Unlock()
	b.cond.Broadcast()
}

// SetDoneWithError signals that the writer finished with an error; the next Read after drain returns that error.
func (b *SafeBuffer) SetDoneWithError(err error) {
	b.mu.Lock()
	b.done = true
	b.doneErr = err
	b.mu.Unlock()
	b.cond.Broadcast()
}

// Close marks the buffer closed and wakes any goroutine parked in Write (at the cap) or Read (empty
// and not yet done), which then returns io.ErrClosedPipe. Used by Sync to unblock in-flight
// prefetch goroutines on teardown so they don't leak.
func (b *SafeBuffer) Close() {
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
	b.cond.Broadcast()
}

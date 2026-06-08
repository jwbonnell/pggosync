package sync

import (
	"io"
	"sync"
)

// SafeBuffer is a concurrent-safe streaming buffer. A writer goroutine calls
// Write and then SetDone (or SetDoneWithError) when finished. A reader goroutine
// calls Read and receives io.EOF once all data has been consumed and SetDone was
// called. If the writer encountered an error, Read returns that error instead.
type SafeBuffer struct {
	mu      sync.Mutex
	cond    *sync.Cond
	buf     io.ReadWriter
	done    bool
	doneErr error
}

func NewSafeBuffer(buf io.ReadWriter) *SafeBuffer {
	sb := &SafeBuffer{buf: buf}
	sb.cond = sync.NewCond(&sb.mu)
	return sb
}

func (b *SafeBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n, err = b.buf.Write(p)
	b.cond.Signal()
	return
}

func (b *SafeBuffer) Read(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for {
		n, err = b.buf.Read(p)
		if err != io.EOF {
			return n, err
		}
		if b.done {
			if b.doneErr != nil {
				return 0, b.doneErr
			}
			return 0, io.EOF
		}
		b.cond.Wait()
	}
}

func (b *SafeBuffer) SetDone() {
	b.mu.Lock()
	b.done = true
	b.mu.Unlock()
	b.cond.Signal()
}

func (b *SafeBuffer) SetDoneWithError(err error) {
	b.mu.Lock()
	b.done = true
	b.doneErr = err
	b.mu.Unlock()
	b.cond.Signal()
}

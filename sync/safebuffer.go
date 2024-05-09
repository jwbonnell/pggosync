package sync

import (
	"io"
	"sync"
)

type SafeBuffer struct {
	buffer        io.ReadWriter
	mu            *sync.RWMutex
	done          bool
	awaitingWrite bool
	doneChan      chan struct{}
}

func NewSafeBuffer(buffer io.ReadWriter) *SafeBuffer {
	return &SafeBuffer{buffer: buffer, mu: &sync.RWMutex{}, doneChan: make(chan struct{})}
}

func (b *SafeBuffer) SetDone() {
	b.done = true
	b.doneChan <- struct{}{}
}

func (b *SafeBuffer) Read(p []byte) (n int, err error) {
	b.mu.RLock()
	n, err = b.buffer.Read(p)
	b.mu.RUnlock()
	if err == io.EOF && !b.done {
		b.awaitingWrite = true
		<-b.doneChan
		b.awaitingWrite = false
		return b.Read(p)
	}
	return n, err
}

func (b *SafeBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n, err = b.buffer.Write(p)
	if b.awaitingWrite {
		b.doneChan <- struct{}{}
	}
	return n, err
}

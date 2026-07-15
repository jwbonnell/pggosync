package sync

import (
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSafeBuffer_WriteRead(t *testing.T) {
	buf := NewSafeBuffer(0)

	go func() {
		buf.Write([]byte("hello"))
		buf.Write([]byte(" world"))
		buf.SetDone()
	}()

	out, err := io.ReadAll(buf)
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(out))
}

func TestSafeBuffer_SetDoneBeforeRead(t *testing.T) {
	buf := NewSafeBuffer(0)
	buf.Write([]byte("data"))
	buf.SetDone()

	out, err := io.ReadAll(buf)
	require.NoError(t, err)
	assert.Equal(t, "data", string(out))
}

func TestSafeBuffer_SetDoneWithError(t *testing.T) {
	buf := NewSafeBuffer(0)

	sentinel := errors.New("source copy failed")
	go func() {
		buf.Write([]byte("partial"))
		buf.SetDoneWithError(sentinel)
	}()

	p := make([]byte, 1024)
	// First read gets the partial data.
	n, err := buf.Read(p)
	assert.Equal(t, "partial", string(p[:n]))
	assert.NoError(t, err)

	// Second read returns the error.
	_, err = buf.Read(p)
	assert.ErrorIs(t, err, sentinel)
}

func TestSafeBuffer_EmptySetDone(t *testing.T) {
	// SetDone called with nothing written — reader should get immediate EOF.
	buf := NewSafeBuffer(0)
	buf.SetDone()

	out, err := io.ReadAll(buf)
	require.NoError(t, err)
	assert.Empty(t, out)
}

// TestSafeBuffer_Bounded verifies backpressure: once the cap is reached, a Write blocks until a
// Read drains the buffer below the cap.
func TestSafeBuffer_Bounded(t *testing.T) {
	buf := NewSafeBuffer(4) // 4-byte cap

	// Fill to the cap; this write does not exceed it, so it must not block.
	n, err := buf.Write([]byte("abcd"))
	require.NoError(t, err)
	require.Equal(t, 4, n)

	// A second write is at/over the cap and must block until the reader drains.
	wrote := make(chan struct{})
	go func() {
		buf.Write([]byte("ef"))
		close(wrote)
	}()

	// The blocked write must not complete on its own.
	select {
	case <-wrote:
		t.Fatal("Write returned before the buffer was drained below the cap")
	case <-time.After(50 * time.Millisecond):
	}

	// Draining below the cap unblocks the writer.
	p := make([]byte, 4)
	rn, err := buf.Read(p)
	require.NoError(t, err)
	require.Equal(t, "abcd", string(p[:rn]))

	select {
	case <-wrote:
	case <-time.After(time.Second):
		t.Fatal("Write did not unblock after the buffer drained")
	}

	// The remaining bytes are still readable.
	buf.SetDone()
	rest, err := io.ReadAll(buf)
	require.NoError(t, err)
	assert.Equal(t, "ef", string(rest))
}

// TestSafeBuffer_CloseUnblocksWriter verifies Close wakes a writer parked at the cap and makes it
// return io.ErrClosedPipe — the teardown path that prevents leaked prefetch goroutines.
func TestSafeBuffer_CloseUnblocksWriter(t *testing.T) {
	buf := NewSafeBuffer(4)

	_, err := buf.Write([]byte("abcd")) // fill to cap
	require.NoError(t, err)

	writeErr := make(chan error, 1)
	go func() {
		_, err := buf.Write([]byte("ef")) // blocks at the cap
		writeErr <- err
	}()

	// Give the writer a moment to park at the cap, then close.
	select {
	case <-writeErr:
		t.Fatal("Write returned before Close")
	case <-time.After(50 * time.Millisecond):
	}
	buf.Close()

	select {
	case err := <-writeErr:
		assert.ErrorIs(t, err, io.ErrClosedPipe)
	case <-time.After(time.Second):
		t.Fatal("Close did not unblock the parked writer")
	}
}

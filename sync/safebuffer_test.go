package sync

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSafeBuffer_WriteRead(t *testing.T) {
	buf := NewSafeBuffer(&bytes.Buffer{})

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
	buf := NewSafeBuffer(&bytes.Buffer{})
	buf.Write([]byte("data"))
	buf.SetDone()

	out, err := io.ReadAll(buf)
	require.NoError(t, err)
	assert.Equal(t, "data", string(out))
}

func TestSafeBuffer_SetDoneWithError(t *testing.T) {
	buf := NewSafeBuffer(&bytes.Buffer{})

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
	buf := NewSafeBuffer(&bytes.Buffer{})
	buf.SetDone()

	out, err := io.ReadAll(buf)
	require.NoError(t, err)
	assert.Empty(t, out)
}

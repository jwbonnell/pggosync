package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

type TestPathHandler struct{}

func (t TestPathHandler) UserConfigDir() (string, error) {
	return "../.test-config", nil
}

func cleanupTest() {
	_ = os.RemoveAll("../.test-config")
}

func TestConfigHandler_InitConnection(t *testing.T) {
	defer cleanupTest()
	handler := NewUserConfigHandler(TestPathHandler{})
	err := handler.InitConnection("taco")
	assert.NoError(t, err)

	conn, err := handler.GetConnection("taco")
	assert.NoError(t, err)
	assert.Equal(t, "localhost", conn.Host)
	assert.Equal(t, 5444, conn.Port)
}

func TestConfigHandler_GetConnection_Missing(t *testing.T) {
	defer cleanupTest()
	handler := NewUserConfigHandler(TestPathHandler{})
	_, err := handler.GetConnection("doesnotexist")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "doesnotexist")
	assert.Contains(t, err.Error(), "pggosync conn init")
}

func TestConfigHandler_ListConnections(t *testing.T) {
	tests := []struct {
		name        string
		connNames   []string
		expected    []string
		expectedErr assert.ErrorAssertionFunc
	}{
		{
			name:        "single_connection",
			connNames:   []string{"something"},
			expected:    []string{"something"},
			expectedErr: assert.NoError,
		},
		{
			name:        "no_connections",
			connNames:   []string{},
			expected:    nil,
			expectedErr: assert.NoError,
		},
		{
			name:        "multiple_connections",
			connNames:   []string{"burrito", "enchilada", "taco"},
			expected:    []string{"burrito", "enchilada", "taco"},
			expectedErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer cleanupTest()
			handler := &UserConfigHandler{PathHandler: TestPathHandler{}}

			dir, err := handler.PathHandler.UserConfigDir()
			assert.NoError(t, err)
			err = os.MkdirAll(filepath.Join(dir, "pggosync"), 0700)
			assert.NoError(t, err)

			for _, name := range tt.connNames {
				err := handler.InitConnection(name)
				assert.NoError(t, err)
			}
			got, err := handler.ListConnections()
			if !tt.expectedErr(t, err, fmt.Sprintf("unexpected error")) {
				return
			}
			assert.True(t, slices.Equal(tt.expected, got), "listed connections should match expected")
		})
	}
}

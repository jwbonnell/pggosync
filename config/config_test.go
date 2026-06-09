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

func TestConfigHandler_SetGetDefaults(t *testing.T) {
	defer cleanupTest()
	handler := NewUserConfigHandler(TestPathHandler{})
	err := handler.SetDefaults("src", "dst")
	assert.NoError(t, err)

	d, err := handler.GetDefaults()
	assert.NoError(t, err)
	assert.Equal(t, "src", d.Source)
	assert.Equal(t, "dst", d.Dest)
}

func TestConfigHandler_InitConnection(t *testing.T) {
	defer cleanupTest()
	handler := NewUserConfigHandler(TestPathHandler{})
	err := handler.InitConnection("taco")
	assert.NoError(t, err)

	conn, err := handler.GetConnection("taco")
	assert.NoError(t, err)
	assert.Equal(t, "localhost", conn.Host)
	assert.Equal(t, 5432, conn.Port)

	// Defaults should have been set automatically on first init.
	d, err := handler.GetDefaults()
	assert.NoError(t, err)
	assert.Equal(t, "taco", d.Source)
	assert.Equal(t, "taco", d.Dest)
}

func TestConfigHandler_GetConnection_Missing(t *testing.T) {
	defer cleanupTest()
	handler := NewUserConfigHandler(TestPathHandler{})
	_, err := handler.GetConnection("doesnotexist")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "doesnotexist")
	assert.Contains(t, err.Error(), "pggosync init")
}

func TestConfigHandler_UpdateDefaults(t *testing.T) {
	defer cleanupTest()
	handler := NewUserConfigHandler(TestPathHandler{})
	err := handler.SetDefaults("a", "b")
	assert.NoError(t, err)

	err = handler.SetDefaults("updated-src", "updated-dst")
	assert.NoError(t, err)

	d, err := handler.GetDefaults()
	assert.NoError(t, err)
	assert.Equal(t, "updated-src", d.Source)
	assert.Equal(t, "updated-dst", d.Dest)
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

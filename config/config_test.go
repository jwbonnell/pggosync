package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// TestConfigHandler_ListConnections_SkipsPrefsKeepsDotted guards M5: prefs.yaml must not be reported
// as a connection, and a connection whose name contains a dot must remain visible.
func TestConfigHandler_ListConnections_SkipsPrefsKeepsDotted(t *testing.T) {
	defer cleanupTest()
	handler := &UserConfigHandler{PathHandler: TestPathHandler{}}

	require.NoError(t, handler.InitConnection("normal"))
	require.NoError(t, handler.InitConnection("my.db"))
	// SavePrefs writes prefs.yaml into the same directory as connections.
	require.NoError(t, handler.SavePrefs(Prefs{}))

	got, err := handler.ListConnections()
	require.NoError(t, err)
	assert.Contains(t, got, "normal")
	assert.Contains(t, got, "my.db")
	assert.NotContains(t, got, "prefs")
}

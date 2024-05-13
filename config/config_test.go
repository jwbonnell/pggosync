package config

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"log"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

type TestPathHandler struct{}

func (t TestPathHandler) UserConfigDir() (string, error) {
	return "../.test-config", nil
}

func cleanupTest() {
	err := os.RemoveAll("../.test-config")
	if err != nil {
		log.Fatalf("Failed to cleanup test: %v", err)
	}
}

func TestConfigHandler_SetGetDefault(t *testing.T) {
	defer cleanupTest()
	handler := NewConfigHandler(TestPathHandler{})
	err := handler.SetDefault("burrito")
	assert.NoError(t, err)

	def, err := handler.GetDefault()
	assert.NoError(t, err)
	assert.Equal(t, "burrito", def, "expected default value to be 'burrito'")
}

func TestConfigHandler_InitConfig(t *testing.T) {
	defer cleanupTest()
	handler := NewConfigHandler(TestPathHandler{})
	err := handler.InitConfig("taco")
	assert.NoError(t, err)
	config, err := handler.GetConfig("taco")
	assert.NoError(t, err)

	_, ok := config.Groups["country"]
	assert.True(t, ok)
}

func TestConfigHandler_UpdateDefault(t *testing.T) {
	defer cleanupTest()
	handler := NewConfigHandler(TestPathHandler{})
	err := handler.SetDefault("something")
	assert.NoError(t, err)

	err = handler.SetDefault("updated")
	assert.NoError(t, err)

	def, err := handler.GetDefault()
	assert.NoError(t, err)
	assert.Equal(t, "updated", def, "expected default value to be 'updated'")
}

func TestConfigHandler_ListConfigs(t *testing.T) {
	tests := []struct {
		name        string
		configIDs   []string
		expected    []string
		expectedErr assert.ErrorAssertionFunc
	}{
		{
			name:        "single_config",
			configIDs:   []string{"something"},
			expected:    []string{"something"},
			expectedErr: assert.NoError,
		},
		{
			name:        "no_configs",
			configIDs:   []string{},
			expected:    []string{},
			expectedErr: assert.NoError,
		},
		{
			name:        "single_config",
			configIDs:   []string{"burrito", "enchilada", "taco"},
			expected:    []string{"burrito", "enchilada", "taco"},
			expectedErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer cleanupTest()
			handler := &ConfigHandler{
				PathHandler: TestPathHandler{},
			}

			dir, err := handler.PathHandler.UserConfigDir()
			assert.NoError(t, err)
			configPath := filepath.Join(dir, "pggosync") + "/"
			err = os.MkdirAll(filepath.Dir(configPath), 0700)
			assert.NoError(t, err)

			for i := range tt.configIDs {
				err := handler.InitConfig(tt.configIDs[i])
				assert.NoError(t, err)
			}
			got, err := handler.ListConfigs()
			if !tt.expectedErr(t, err, fmt.Sprintf("there should be no error here")) {
				return
			}

			assert.True(t, slices.Equal(tt.expected, got), "the listed configs should match the expected list")
		})
	}
}

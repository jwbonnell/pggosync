package config

import (
	"github.com/stretchr/testify/assert"
	"log"
	"os"
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
	config, err := handler.getConfig("taco")
	assert.NoError(t, err)

	_, ok := config.Groups["country"]
	assert.True(t, ok)
}

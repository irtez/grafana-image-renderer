package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func runCaptureConfig(t *testing.T) (CaptureConfig, error) {
	t.Helper()

	var cfg CaptureConfig
	cmd := &cli.Command{
		Flags: CaptureFlags(),
		Action: func(_ context.Context, c *cli.Command) error {
			var err error
			cfg, err = CaptureConfigFromCommand(c)
			return err
		},
		Reader:    nopReader{},
		Writer:    nopWriter{},
		ErrWriter: nopWriter{},
	}

	err := cmd.Run(t.Context(), []string{"renderer"})
	return cfg, err
}

func TestCaptureConfigDefaults(t *testing.T) {
	cfg, err := runCaptureConfig(t)

	require.NoError(t, err)
	require.False(t, cfg.SemanticEnabled)
	require.Equal(t, 5*time.Second, cfg.Timeout)
	require.Equal(t, 1048576, cfg.MaxJSONBytes)
}

func TestCaptureConfigEnvAliases(t *testing.T) {
	t.Setenv("CAPTURE_SEMANTIC_ENABLED", "true")
	t.Setenv("CAPTURE_TIMEOUT", "7s")
	t.Setenv("CAPTURE_MAX_JSON_BYTES", "2048")

	cfg, err := runCaptureConfig(t)

	require.NoError(t, err)
	require.True(t, cfg.SemanticEnabled)
	require.Equal(t, 7*time.Second, cfg.Timeout)
	require.Equal(t, 2048, cfg.MaxJSONBytes)
}

func TestCaptureConfigYAML(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile(filepath.Join("config.yaml"), []byte(`capture:
  semantic-enabled: true
  timeout: 9s
  max-json-bytes: 4096
`), 0o600))

	cfg, err := runCaptureConfig(t)

	require.NoError(t, err)
	require.True(t, cfg.SemanticEnabled)
	require.Equal(t, 9*time.Second, cfg.Timeout)
	require.Equal(t, 4096, cfg.MaxJSONBytes)
}

func TestCaptureConfigRejectsNonPositiveLimits(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		value   string
		message string
	}{
		{name: "zero timeout", env: "CAPTURE_TIMEOUT", value: "0s", message: "capture timeout must be positive"},
		{name: "negative timeout", env: "CAPTURE_TIMEOUT", value: "-1s", message: "capture timeout must be positive"},
		{name: "zero byte limit", env: "CAPTURE_MAX_JSON_BYTES", value: "0", message: "capture max-json-bytes must be positive"},
		{name: "negative byte limit", env: "CAPTURE_MAX_JSON_BYTES", value: "-1", message: "capture max-json-bytes must be positive"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.env, tt.value)

			_, err := runCaptureConfig(t)

			require.ErrorContains(t, err, tt.message)
		})
	}
}

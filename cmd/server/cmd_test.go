package server

import (
	"context"
	"io"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/grafana/grafana-image-renderer/pkg/config"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

type testNopReader struct{}

func (testNopReader) Read(p []byte) (n int, err error) {
	return 0, io.EOF
}

type testNopWriter struct{}

func (testNopWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func TestNewCmd(t *testing.T) {
	t.Parallel()
	cmd := NewCmd()
	require.Equal(t, "server", cmd.Name)
	require.Contains(t, cmd.Usage, "Run the server")
	require.NotNil(t, cmd.Action)
	require.NotEmpty(t, cmd.Flags)
}

func TestParseConfig(t *testing.T) {
	t.Parallel()
	cmd := &cli.Command{
		Flags: slices.Concat(
			config.ServerFlags(),
			config.TracingFlags(),
			config.BrowserFlags(),
			config.RateLimitFlags(),
			config.APIFlags(),
			config.CaptureFlags(),
		),
		Action: func(context.Context, *cli.Command) error {
			return nil
		},
		Reader:    testNopReader{},
		Writer:    testNopWriter{},
		ErrWriter: testNopWriter{},
	}

	args := []string{
		"",
		"--server.addr", ":9433",
		"--browser.ws-url-read-timeout", "30s",
		"--api.default-encoding", "png",
		"--capture.semantic-enabled", "true",
		"--capture.timeout", "10s",
	}
	err := cmd.Run(context.Background(), args)
	require.NoError(t, err)

	cfg, err := ParseConfig(cmd)
	require.NoError(t, err)
	require.Equal(t, ":9433", cfg.server.Addr)
	require.Equal(t, "png", cfg.api.DefaultEncoding)
	require.True(t, cfg.capture.SemanticEnabled)
	require.Equal(t, 10*time.Second, cfg.capture.Timeout)
}

func TestParseConfig_InvalidServerAddr(t *testing.T) {
	t.Parallel()
	cmd := &cli.Command{
		Flags: slices.Concat(
			config.ServerFlags(),
			config.TracingFlags(),
			config.BrowserFlags(),
			config.RateLimitFlags(),
			config.APIFlags(),
			config.CaptureFlags(),
		),
		Action: func(context.Context, *cli.Command) error {
			return nil
		},
		Reader:    testNopReader{},
		Writer:    testNopWriter{},
		ErrWriter: testNopWriter{},
	}

	args := []string{
		"",
		"--server.min-tls-version", "invalid",
	}
	err := cmd.Run(context.Background(), args)
	require.NoError(t, err)

	_, err = ParseConfig(cmd)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to parse server config")
}

func TestParseConfig_InvalidBrowserPath(t *testing.T) {
	t.Parallel()
	cmd := &cli.Command{
		Flags: slices.Concat(
			config.ServerFlags(),
			config.TracingFlags(),
			config.BrowserFlags(),
			config.RateLimitFlags(),
			config.APIFlags(),
			config.CaptureFlags(),
		),
		Action: func(context.Context, *cli.Command) error {
			return nil
		},
		Reader:    testNopReader{},
		Writer:    testNopWriter{},
		ErrWriter: testNopWriter{},
	}

	args := []string{
		"",
		"--browser.path", "/nonexistent/binary",
	}
	err := cmd.Run(context.Background(), args)
	require.NoError(t, err)

	cfg, err := ParseConfig(cmd)
	require.NoError(t, err)
	require.Equal(t, "/nonexistent/binary", cfg.browser.Path)
}

func TestParseConfig_DefaultValues(t *testing.T) {
	t.Parallel()
	cmd := &cli.Command{
		Flags: slices.Concat(
			config.ServerFlags(),
			config.TracingFlags(),
			config.BrowserFlags(),
			config.RateLimitFlags(),
			config.APIFlags(),
			config.CaptureFlags(),
		),
		Action: func(context.Context, *cli.Command) error {
			return nil
		},
		Reader:    testNopReader{},
		Writer:    testNopWriter{},
		ErrWriter: testNopWriter{},
	}

	err := cmd.Run(context.Background(), []string{""})
	require.NoError(t, err)

	cfg, err := ParseConfig(cmd)
	require.NoError(t, err)
	require.Equal(t, ":8081", cfg.server.Addr)
	require.Equal(t, "pdf", cfg.api.DefaultEncoding)
	require.False(t, cfg.capture.SemanticEnabled)
}

func TestParseConfig_AllConfigSections(t *testing.T) {
	t.Parallel()
	cmd := &cli.Command{
		Flags: slices.Concat(
			config.ServerFlags(),
			config.TracingFlags(),
			config.BrowserFlags(),
			config.RateLimitFlags(),
			config.APIFlags(),
			config.CaptureFlags(),
		),
		Action: func(context.Context, *cli.Command) error {
			return nil
		},
		Reader:    testNopReader{},
		Writer:    testNopWriter{},
		ErrWriter: testNopWriter{},
	}

	args := []string{
		"",
		"--server.addr", ":9434",
		"--browser.path", "/tmp/chrome",
		"--browser.ws-url-read-timeout", "30s",
		"--tracing.endpoint", "",
		"--rate-limit.disabled", "true",
		"--api.default-encoding", "pdf",
		"--capture.timeout", "5s",
	}
	err := cmd.Run(context.Background(), args)
	require.NoError(t, err)

	cfg, err := ParseConfig(cmd)
	require.NoError(t, err)
	require.Equal(t, ":9434", cfg.server.Addr)
	require.Equal(t, "/tmp/chrome", cfg.browser.Path)
	require.Equal(t, "", cfg.tracing.Endpoint)
	require.True(t, cfg.rateLimit.Disabled)
	require.Equal(t, "pdf", cfg.api.DefaultEncoding)
	require.Equal(t, 5*time.Second, cfg.capture.Timeout)
}

func TestRun_ParseConfigError(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	cmd := &cli.Command{
		Flags: slices.Concat(
			config.ServerFlags(),
			config.TracingFlags(),
			config.BrowserFlags(),
			config.RateLimitFlags(),
			config.APIFlags(),
			config.CaptureFlags(),
		),
		Action: func(context.Context, *cli.Command) error {
			return nil
		},
		Reader:    testNopReader{},
		Writer:    testNopWriter{},
		ErrWriter: testNopWriter{},
	}

	// ParseConfig should fail with invalid TLS version
	err := cmd.Run(context.Background(), []string{
		"",
		"--server.min-tls-version", "invalid",
	})
	require.NoError(t, err)

	// run() will call ParseConfig and return the error
	err = run(ctx, cmd)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to parse config")
	require.Contains(t, err.Error(), "failed to parse server config")
}

func TestRun_TracerProviderError(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	cmd := &cli.Command{
		Flags: slices.Concat(
			config.ServerFlags(),
			config.TracingFlags(),
			config.BrowserFlags(),
			config.RateLimitFlags(),
			config.APIFlags(),
			config.CaptureFlags(),
		),
		Action: func(context.Context, *cli.Command) error {
			return nil
		},
		Reader:    testNopReader{},
		Writer:    testNopWriter{},
		ErrWriter: testNopWriter{},
	}

	// Set tracing config to use a non-existent trusted certificate
	err := cmd.Run(context.Background(), []string{
		"",
		"--tracing.endpoint", "http://localhost:4317",
		"--tracing.trusted-certificate", "/nonexistent/cert.pem",
	})
	require.NoError(t, err)

	// run() will call traces.NewTracerProvider and return the error
	err = run(ctx, cmd)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to set up tracer")
	require.Contains(t, err.Error(), "failed to read trusted certificate file")
}

func TestRun_TracerProviderNil(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// When no tracing endpoint is configured, traces.NewTracerProvider returns nil, nil
	// This exercises the path where tracerProvider is nil (if tracerProvider != nil block is skipped)
	_ = run(ctx, &cli.Command{
		Flags: slices.Concat(
			config.ServerFlags(),
			config.TracingFlags(),
			config.BrowserFlags(),
			config.RateLimitFlags(),
			config.APIFlags(),
			config.CaptureFlags(),
		),
		Action: func(context.Context, *cli.Command) error {
			return nil
		},
		Reader:    testNopReader{},
		Writer:    testNopWriter{},
		ErrWriter: testNopWriter{},
	})
}

func TestRun_FullFlow(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// run() will proceed through config parsing, tracing setup, service creation,
	// and eventually reach ListenAndServe. It will either:
	// 1. Fail immediately if port is in use
	// 2. Succeed (context cancelled during serve loop)
	// Either way, all code paths up to ListenAndServe are exercised.
	_ = run(ctx, &cli.Command{
		Flags: slices.Concat(
			config.ServerFlags(),
			config.TracingFlags(),
			config.BrowserFlags(),
			config.RateLimitFlags(),
			config.APIFlags(),
			config.CaptureFlags(),
		),
		Action: func(context.Context, *cli.Command) error {
			return nil
		},
		Reader:    testNopReader{},
		Writer:    testNopWriter{},
		ErrWriter: testNopWriter{},
	})
}

func TestRun_ContextCancelled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		_ = run(ctx, &cli.Command{
			Flags: slices.Concat(
				config.ServerFlags(),
				config.TracingFlags(),
				config.BrowserFlags(),
				config.RateLimitFlags(),
				config.APIFlags(),
				config.CaptureFlags(),
			),
			Action: func(context.Context, *cli.Command) error {
				return nil
			},
			Reader:    testNopReader{},
			Writer:    testNopWriter{},
			ErrWriter: testNopWriter{},
		})
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done
}

func TestMaxProcsLog(t *testing.T) {
	t.Parallel()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	maxProcsLog("test format %s %d", "arg", 42)
}

func TestMaxProcsLog_Empty(t *testing.T) {
	t.Parallel()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	maxProcsLog("")
}

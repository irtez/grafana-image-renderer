package service_test

import (
	"runtime/debug"
	"sync"
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/grafana/grafana-image-renderer/pkg/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrettyVersion(t *testing.T) {
	t.Parallel()
	if !isGitVcs(t) {
		t.Skip("skipping test, not built with -buildvcs")
	}

	svc := service.NewVersionService()
	version := svc.GetPrettyVersion()
	require.NotEmpty(t, version, "expected service version to be non-empty")
}

func TestRenderVersion(t *testing.T) {
	t.Parallel()

	svc := service.NewVersionService()
	version := svc.GetRenderVersion()
	_, err := semver.NewVersion(version)
	require.NoError(t, err, "expected render version to be valid semver")
}

func isGitVcs(tb testing.TB) bool {
	tb.Helper()

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return false
	}

	for _, setting := range info.Settings {
		if setting.Key == "vcs" {
			return setting.Value == "git"
		}
	}
	return false
}

func TestVersionService_NewVersionService(t *testing.T) {
	t.Parallel()
	svc := service.NewVersionService()
	require.NotNil(t, svc)
}

func TestVersionService_GetRenderVersion(t *testing.T) {
	t.Parallel()
	svc := service.NewVersionService()
	version := svc.GetRenderVersion()
	require.Equal(t, "5.0.0", version)
}

func TestVersionService_GetPrettyVersion(t *testing.T) {
	t.Parallel()
	svc := service.NewVersionService()
	version := svc.GetPrettyVersion()
	assert.NotEmpty(t, version)
}

func TestVersionService_GetPrettyVersion_ThreadSafe(t *testing.T) {
	t.Parallel()
	svc := service.NewVersionService()
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = svc.GetPrettyVersion()
		}()
	}
	wg.Wait()
}

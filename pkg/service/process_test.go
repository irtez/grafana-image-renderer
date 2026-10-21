package service

import (
	"testing"
	"time"

	"github.com/grafana/grafana-image-renderer/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestNewProcessStatService(t *testing.T) {
	t.Parallel()
	cfg := config.RateLimitConfig{TrackerInterval: time.Second, TrackerDecay: 2}
	svc := NewProcessStatService(cfg)
	require.Equal(t, cfg, svc.cfg)
	require.NotNil(t, svc.log)
	require.Equal(t, int64(0), svc.MaxMemory)
	require.Equal(t, int64(0), svc.PeakMemory)
}

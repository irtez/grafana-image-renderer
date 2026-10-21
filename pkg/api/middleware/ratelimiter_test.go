package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/grafana-image-renderer/pkg/config"
	"github.com/grafana/grafana-image-renderer/pkg/service"
	"github.com/stretchr/testify/require"
)

func TestNewRateLimiter_Disabled(t *testing.T) {
	t.Parallel()
	limiter, err := NewRateLimiter(nil, config.RateLimitConfig{Disabled: true})
	require.NoError(t, err)
	require.NotNil(t, limiter)
}

func TestNewRateLimiter_Enabled(t *testing.T) {
	t.Parallel()
	svc := &service.ProcessStatService{}
	limiter, err := NewRateLimiter(svc, config.RateLimitConfig{MinLimit: 1, MaxLimit: 100})
	require.NoError(t, err)
	require.NotNil(t, limiter)
}

func TestRateLimiter_Limit(t *testing.T) {
	t.Parallel()
	svc := &service.ProcessStatService{}
	limiter, err := NewRateLimiter(svc, config.RateLimitConfig{MinLimit: 1, MaxLimit: 100})
	require.NoError(t, err)
	handler := limiter.Limit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestNoOpLimiter_Limit(t *testing.T) {
	t.Parallel()
	limiter := noOpLimiter{}
	handler := limiter.Limit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)
}

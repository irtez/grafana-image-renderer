package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/grafana-image-renderer/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestRequestLogger_SilencePath(t *testing.T) {
	t.Parallel()
	handler := RequestLogger(config.APIConfig{APISilenceRequestLogPath: []string{"/healthz"}}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestRequestLogger_NoSilence(t *testing.T) {
	t.Parallel()
	handler := RequestLogger(config.APIConfig{}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/other", nil))
	require.Equal(t, http.StatusCreated, rec.Code)
}

func TestRequestLogger_ResponseWriter(t *testing.T) {
	t.Parallel()
	handler := RequestLogger(config.APIConfig{}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("test"))
	}))
	req := httptest.NewRequest("GET", "/test", nil)
	wrapped := httptest.NewRecorder()
	handler.ServeHTTP(wrapped, req)
	require.Equal(t, http.StatusAccepted, wrapped.Code)
}

func TestRequestLogger_WithSilencePaths(t *testing.T) {
	t.Parallel()
	apiConfig := config.APIConfig{APISilenceRequestLogPath: []string{"/healthz", "/metrics"}}
	_ = apiConfig // используем для создания handler
	handler := RequestLogger(apiConfig, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestRequestMetrics_InFlight(t *testing.T) {
	t.Parallel()
	handler := InFlightMetrics(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestTracing_Handler(t *testing.T) {
	t.Parallel()
	handler := Tracing(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

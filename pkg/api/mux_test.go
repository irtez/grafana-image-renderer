package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/grafana/grafana-image-renderer/pkg/api"
	"github.com/grafana/grafana-image-renderer/pkg/api/middleware"
	"github.com/grafana/grafana-image-renderer/pkg/capture"
	"github.com/grafana/grafana-image-renderer/pkg/config"
	"github.com/grafana/grafana-image-renderer/pkg/service"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

type mockRenderer struct {
	renderFn func(ctx context.Context, url string, printer service.Printer, opts ...service.RenderingOption) ([]byte, string, error)
}

func (m *mockRenderer) Render(ctx context.Context, url string, printer service.Printer, opts ...service.RenderingOption) ([]byte, string, error) {
	if m.renderFn != nil {
		return m.renderFn(ctx, url, printer, opts...)
	}
	return []byte("mock"), "text/plain", nil
}

type testCollector struct{}

func (testCollector) Kind() string {
	return "test"
}

func (testCollector) Collect(ctx context.Context, req capture.Request, panelID int) (capture.Collection, error) {
	return capture.Collection{Payload: struct{ Kind string }{Kind: "test"}}, nil
}

func newTestEngine(t *testing.T) *capture.Engine {
	t.Helper()
	engine, err := capture.NewEngine(config.CaptureConfig{SemanticEnabled: true, Timeout: time.Second, MaxJSONBytes: 4096}, testCollector{})
	require.NoError(t, err)
	return engine
}

func TestHandleGetRender_MissingURL(t *testing.T) {
	handler := api.HandleGetRender(&mockRenderer{}, config.APIConfig{}, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render", nil))
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleGetRender_InvalidWidth(t *testing.T) {
	handler := api.HandleGetRender(&mockRenderer{}, config.APIConfig{}, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com&width=abc", nil))
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleGetRender_InvalidHeight(t *testing.T) {
	handler := api.HandleGetRender(&mockRenderer{}, config.APIConfig{}, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com&height=abc", nil))
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleGetRender_InvalidTimeout(t *testing.T) {
	handler := api.HandleGetRender(&mockRenderer{}, config.APIConfig{}, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com&timeout=abc", nil))
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleGetRender_InvalidScaleFactor(t *testing.T) {
	handler := api.HandleGetRender(&mockRenderer{}, config.APIConfig{}, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com&deviceScaleFactor=abc", nil))
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleGetRender_InvalidEncoding(t *testing.T) {
	handler := api.HandleGetRender(&mockRenderer{}, config.APIConfig{}, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com&encoding=bad", nil))
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleGetRender_InvalidPDFFormat(t *testing.T) {
	handler := api.HandleGetRender(&mockRenderer{}, config.APIConfig{}, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com&encoding=pdf&pdf.format=bad", nil))
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleGetRender_WithValidParams(t *testing.T) {
	engine, _ := capture.NewEngine(config.CaptureConfig{SemanticEnabled: false, Timeout: time.Second, MaxJSONBytes: 4096}, testCollector{})
	handler := api.HandleGetRender(&mockRenderer{renderFn: func(ctx context.Context, url string, printer service.Printer, opts ...service.RenderingOption) ([]byte, string, error) {
		return []byte("png"), "image/png", nil
	}}, config.APIConfig{}, engine)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com&encoding=png", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleGetRender_WithPDFEncoding(t *testing.T) {
	engine, _ := capture.NewEngine(config.CaptureConfig{SemanticEnabled: false, Timeout: time.Second, MaxJSONBytes: 4096}, testCollector{})
	handler := api.HandleGetRender(&mockRenderer{renderFn: func(ctx context.Context, url string, printer service.Printer, opts ...service.RenderingOption) ([]byte, string, error) {
		return []byte("pdf"), "application/pdf", nil
	}}, config.APIConfig{}, engine)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com&encoding=pdf", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleGetRender_WithPNGEncoding(t *testing.T) {
	engine, _ := capture.NewEngine(config.CaptureConfig{SemanticEnabled: false, Timeout: time.Second, MaxJSONBytes: 4096}, testCollector{})
	handler := api.HandleGetRender(&mockRenderer{renderFn: func(ctx context.Context, url string, printer service.Printer, opts ...service.RenderingOption) ([]byte, string, error) {
		return []byte("png"), "image/png", nil
	}}, config.APIConfig{}, engine)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com&encoding=png", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleGetRender_WithAcceptLanguage(t *testing.T) {
	engine, _ := capture.NewEngine(config.CaptureConfig{SemanticEnabled: false, Timeout: time.Second, MaxJSONBytes: 4096}, testCollector{})
	handler := api.HandleGetRender(&mockRenderer{renderFn: func(ctx context.Context, url string, printer service.Printer, opts ...service.RenderingOption) ([]byte, string, error) {
		return []byte("png"), "image/png", nil
	}}, config.APIConfig{}, engine)
	req := httptest.NewRequest("GET", "/render?url=http://x.com&encoding=png", nil)
	req.Header.Set("Accept-Language", "ru-RU")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	t.Log(rec.Code, rec.Body.String())
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestRequireAuthToken_Missing(t *testing.T) {
	handler := middleware.RequireAuthToken(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), "token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestRequireAuthToken_Valid(t *testing.T) {
	handler := middleware.RequireAuthToken(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), "token")
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Auth-Token", "token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	t.Log("Code:", rec.Code, "Body:", rec.Body.String())
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestTrustedURL_BlocksFile(t *testing.T) {
	handler := middleware.TrustedURL(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/?url=file:///etc/passwd", nil))
	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestTrustedURL_BlocksChrome(t *testing.T) {
	handler := middleware.TrustedURL(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/?url=chrome://version", nil))
	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestTrustedURL_AllowsHTTP(t *testing.T) {
	handler := middleware.TrustedURL(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/?url=http://example.com", nil))
	t.Log("Code:", rec.Code, "Body:", rec.Body.String())
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestTrustedURL_AllowsHTTPS(t *testing.T) {
	handler := middleware.TrustedURL(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/?url=https://example.com", nil))
	t.Log("Code:", rec.Code, "Body:", rec.Body.String())
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestRecovery_Panic(t *testing.T) {
	handler := middleware.Recovery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test")
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestRecovery_NoPanic(t *testing.T) {
	handler := middleware.Recovery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	t.Log("Code:", rec.Code, "Body:", rec.Body.String())
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestInFlightMetrics(t *testing.T) {
	handler := middleware.InFlightMetrics(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	t.Log("Code:", rec.Code, "Body:", rec.Body.String())
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestTracing(t *testing.T) {
	handler := middleware.Tracing(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	t.Log("Code:", rec.Code, "Body:", rec.Body.String())
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestNewHandler_Success(t *testing.T) {
	t.Parallel()
	metrics := prometheus.NewRegistry()
	serverConfig := config.ServerConfig{}
	apiConfig := config.APIConfig{}
	rateLimitConfig := config.RateLimitConfig{}
	processStatService := &service.ProcessStatService{}
	browser := (*service.BrowserService)(nil)
	versions := service.NewVersionService()
	captureEngine := (*capture.Engine)(nil)

	handler, err := api.NewHandler(
		metrics,
		serverConfig,
		apiConfig,
		rateLimitConfig,
		processStatService,
		browser,
		versions,
		captureEngine,
	)
	require.NoError(t, err)
	require.NotNil(t, handler)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "Grafana Image Renderer")
}

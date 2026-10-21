package api_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/grafana/grafana-image-renderer/pkg/api"
	"github.com/grafana/grafana-image-renderer/pkg/capture"
	"github.com/grafana/grafana-image-renderer/pkg/config"
	"github.com/grafana/grafana-image-renderer/pkg/service"
	"github.com/stretchr/testify/require"
)

func TestHandleGetRender_URLParseError(t *testing.T) {
	t.Parallel()
	handler := api.HandleGetRender(&mockRenderer{}, config.APIConfig{}, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com%00invalid", nil))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "invalid 'url'")
}

func TestHandleGetRender_TimeoutWithValue(t *testing.T) {
	t.Parallel()
	handler := api.HandleGetRender(&mockRenderer{renderFn: func(ctx context.Context, url string, printer service.Printer, opts ...service.RenderingOption) ([]byte, string, error) {
		_, ok := ctx.Deadline()
		require.True(t, ok)
		return []byte("ok"), "text/plain", nil
	}}, config.APIConfig{}, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com&encoding=png&timeout=5", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleGetRender_TimeoutWithDuration(t *testing.T) {
	t.Parallel()
	handler := api.HandleGetRender(&mockRenderer{renderFn: func(ctx context.Context, url string, printer service.Printer, opts ...service.RenderingOption) ([]byte, string, error) {
		_, ok := ctx.Deadline()
		require.True(t, ok)
		return []byte("ok"), "text/plain", nil
	}}, config.APIConfig{}, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com&encoding=png&timeout=30s", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleGetRender_TimeoutZeroValue(t *testing.T) {
	t.Parallel()
	handler := api.HandleGetRender(&mockRenderer{renderFn: func(ctx context.Context, url string, printer service.Printer, opts ...service.RenderingOption) ([]byte, string, error) {
		_, ok := ctx.Deadline()
		require.False(t, ok)
		return []byte("ok"), "text/plain", nil
	}}, config.APIConfig{}, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com&encoding=png&timeout=0", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleGetRender_DeviceScaleFactorValid(t *testing.T) {
	t.Parallel()
	handler := api.HandleGetRender(&mockRenderer{renderFn: func(ctx context.Context, url string, printer service.Printer, opts ...service.RenderingOption) ([]byte, string, error) {
		return []byte("ok"), "text/plain", nil
	}}, config.APIConfig{}, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com&encoding=png&deviceScaleFactor=2.0", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleGetRender_TimezoneValid(t *testing.T) {
	t.Parallel()
	handler := api.HandleGetRender(&mockRenderer{renderFn: func(ctx context.Context, url string, printer service.Printer, opts ...service.RenderingOption) ([]byte, string, error) {
		return []byte("ok"), "text/plain", nil
	}}, config.APIConfig{}, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com&encoding=png&timezone=UTC", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleGetRender_TimezoneInvalid(t *testing.T) {
	t.Parallel()
	handler := api.HandleGetRender(&mockRenderer{renderFn: func(ctx context.Context, url string, printer service.Printer, opts ...service.RenderingOption) ([]byte, string, error) {
		return []byte("ok"), "text/plain", nil
	}}, config.APIConfig{}, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com&encoding=png&timezone=InvalidZone", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleGetRender_Landscape(t *testing.T) {
	t.Parallel()
	handler := api.HandleGetRender(&mockRenderer{renderFn: func(ctx context.Context, url string, printer service.Printer, opts ...service.RenderingOption) ([]byte, string, error) {
		return []byte("ok"), "text/plain", nil
	}}, config.APIConfig{}, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com&encoding=png&landscape=true", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, httptest.NewRequest("GET", "/render?url=http://x.com&encoding=png&landscape=false", nil))
	require.Equal(t, http.StatusOK, rec2.Code)
}

func TestHandleGetRender_RenderKeyCookie(t *testing.T) {
	t.Parallel()
	handler := api.HandleGetRender(&mockRenderer{renderFn: func(ctx context.Context, url string, printer service.Printer, opts ...service.RenderingOption) ([]byte, string, error) {
		return []byte("ok"), "text/plain", nil
	}}, config.APIConfig{}, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com&encoding=png&renderKey=abc&domain=grafana", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleGetRender_DefaultEncoding(t *testing.T) {
	t.Parallel()
	apiCfg := config.APIConfig{DefaultEncoding: "png"}
	handler := api.HandleGetRender(&mockRenderer{renderFn: func(ctx context.Context, url string, printer service.Printer, opts ...service.RenderingOption) ([]byte, string, error) {
		return []byte("ok"), "image/png", nil
	}}, apiCfg, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleGetRender_CaptureMatchErrorV2(t *testing.T) {
	t.Parallel()
	engine, err := capture.NewEngine(config.CaptureConfig{SemanticEnabled: true, Timeout: time.Second, MaxJSONBytes: 4096}, testCollector{})
	require.NoError(t, err)
	handler := api.HandleGetRender(&mockRenderer{}, config.APIConfig{}, engine)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com/d/my-uid/my-slug%3FsiamCaptureVersion=2%26siamCaptureKind=svgmodifier%26renderKey=rk%26domain=d&encoding=png&renderKey=rk&domain=d", nil))
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleGetRender_CaptureMatchErrorV1(t *testing.T) {
	t.Parallel()
	engine, err := capture.NewEngine(config.CaptureConfig{SemanticEnabled: true, Timeout: time.Second, MaxJSONBytes: 4096}, testCollector{})
	require.NoError(t, err)
	handler := api.HandleGetRender(&mockRenderer{renderFn: func(ctx context.Context, url string, printer service.Printer, opts ...service.RenderingOption) ([]byte, string, error) {
		return []byte("json"), "application/json", nil
	}}, config.APIConfig{}, engine)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com/d-solo/my-uid/my-slug%3FsiamCaptureVersion=1%26siamCaptureKind=test%26panelId=1&encoding=png&renderKey=rk&domain=d", nil))
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleGetRender_PDFOptions(t *testing.T) {
	t.Parallel()
	engine, err := capture.NewEngine(config.CaptureConfig{SemanticEnabled: false, Timeout: time.Second, MaxJSONBytes: 4096}, testCollector{})
	require.NoError(t, err)
	handler := api.HandleGetRender(&mockRenderer{renderFn: func(ctx context.Context, url string, printer service.Printer, opts ...service.RenderingOption) ([]byte, string, error) {
		return []byte("pdf"), "application/pdf", nil
	}}, config.APIConfig{}, engine)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com&encoding=pdf&pdf.format=a4&pdf.printBackground=true&pdf.pageRanges=1-3&pdf.landscape=true", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleGetRender_PDFPrintBackgroundFromURL(t *testing.T) {
	t.Parallel()
	engine, err := capture.NewEngine(config.CaptureConfig{SemanticEnabled: false, Timeout: time.Second, MaxJSONBytes: 4096}, testCollector{})
	require.NoError(t, err)
	handler := api.HandleGetRender(&mockRenderer{renderFn: func(ctx context.Context, url string, printer service.Printer, opts ...service.RenderingOption) ([]byte, string, error) {
		return []byte("pdf"), "application/pdf", nil
	}}, config.APIConfig{}, engine)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com?pdf.printBackground=true&encoding=pdf", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleGetRender_PDFPageRangesFromURL(t *testing.T) {
	t.Parallel()
	engine, err := capture.NewEngine(config.CaptureConfig{SemanticEnabled: false, Timeout: time.Second, MaxJSONBytes: 4096}, testCollector{})
	require.NoError(t, err)
	handler := api.HandleGetRender(&mockRenderer{renderFn: func(ctx context.Context, url string, printer service.Printer, opts ...service.RenderingOption) ([]byte, string, error) {
		return []byte("pdf"), "application/pdf", nil
	}}, config.APIConfig{}, engine)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com?pdf.pageRanges=1&encoding=pdf", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleGetRender_PDFPaperSizeFromURL(t *testing.T) {
	t.Parallel()
	engine, err := capture.NewEngine(config.CaptureConfig{SemanticEnabled: false, Timeout: time.Second, MaxJSONBytes: 4096}, testCollector{})
	require.NoError(t, err)
	handler := api.HandleGetRender(&mockRenderer{renderFn: func(ctx context.Context, url string, printer service.Printer, opts ...service.RenderingOption) ([]byte, string, error) {
		return []byte("pdf"), "application/pdf", nil
	}}, config.APIConfig{}, engine)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com?pdf.format=a4&encoding=pdf", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleGetRender_PDFLandscapeFromURL(t *testing.T) {
	t.Parallel()
	engine, err := capture.NewEngine(config.CaptureConfig{SemanticEnabled: false, Timeout: time.Second, MaxJSONBytes: 4096}, testCollector{})
	require.NoError(t, err)
	handler := api.HandleGetRender(&mockRenderer{renderFn: func(ctx context.Context, url string, printer service.Printer, opts ...service.RenderingOption) ([]byte, string, error) {
		return []byte("pdf"), "application/pdf", nil
	}}, config.APIConfig{}, engine)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com?pdf.landscape=true&encoding=pdf", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleGetRender_PNGFullHeight(t *testing.T) {
	t.Parallel()
	engine, err := capture.NewEngine(config.CaptureConfig{SemanticEnabled: false, Timeout: time.Second, MaxJSONBytes: 4096}, testCollector{})
	require.NoError(t, err)
	handler := api.HandleGetRender(&mockRenderer{renderFn: func(ctx context.Context, url string, printer service.Printer, opts ...service.RenderingOption) ([]byte, string, error) {
		return []byte("png"), "image/png", nil
	}}, config.APIConfig{}, engine)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com&encoding=png&width=1000", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleGetRender_RenderDeadlineExceeded(t *testing.T) {
	t.Parallel()
	handler := api.HandleGetRender(&mockRenderer{renderFn: func(ctx context.Context, url string, printer service.Printer, opts ...service.RenderingOption) ([]byte, string, error) {
		return nil, "", context.DeadlineExceeded
	}}, config.APIConfig{}, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com&encoding=png", nil))
	require.Equal(t, http.StatusRequestTimeout, rec.Code)
}

func TestHandleGetRender_RenderContextCanceled(t *testing.T) {
	t.Parallel()
	handler := api.HandleGetRender(&mockRenderer{renderFn: func(ctx context.Context, url string, printer service.Printer, opts ...service.RenderingOption) ([]byte, string, error) {
		return nil, "", context.Canceled
	}}, config.APIConfig{}, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com&encoding=png", nil))
	require.Equal(t, http.StatusRequestTimeout, rec.Code)
}

func TestHandleGetRender_RenderReadinessTimeout(t *testing.T) {
	t.Parallel()
	handler := api.HandleGetRender(&mockRenderer{renderFn: func(ctx context.Context, url string, printer service.Printer, opts ...service.RenderingOption) ([]byte, string, error) {
		return nil, "", service.ErrBrowserReadinessTimeout
	}}, config.APIConfig{}, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com&encoding=png", nil))
	require.Equal(t, http.StatusRequestTimeout, rec.Code)
}

func TestHandleGetRender_RenderInvalidBrowserOption(t *testing.T) {
	t.Parallel()
	handler := api.HandleGetRender(&mockRenderer{renderFn: func(ctx context.Context, url string, printer service.Printer, opts ...service.RenderingOption) ([]byte, string, error) {
		return nil, "", service.ErrInvalidBrowserOption
	}}, config.APIConfig{}, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com&encoding=png", nil))
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleGetRender_RenderInternalServerError(t *testing.T) {
	t.Parallel()
	handler := api.HandleGetRender(&mockRenderer{renderFn: func(ctx context.Context, url string, printer service.Printer, opts ...service.RenderingOption) ([]byte, string, error) {
		return nil, "", errors.New("internal error")
	}}, config.APIConfig{}, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com&encoding=png", nil))
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Contains(t, rec.Body.String(), "Failed to render")
}

func TestHandleGetRender_ForcePollingMode(t *testing.T) {
	t.Parallel()
	engine, err := capture.NewEngine(config.CaptureConfig{SemanticEnabled: false, Timeout: time.Second, MaxJSONBytes: 4096}, testCollector{})
	require.NoError(t, err)
	handler := api.HandleGetRender(&mockRenderer{renderFn: func(ctx context.Context, url string, printer service.Printer, opts ...service.RenderingOption) ([]byte, string, error) {
		return []byte("png"), "image/png", nil
	}}, config.APIConfig{}, engine)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com?forcePollingMode=true&encoding=png&x=", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleGetRender_ResponseHeaders(t *testing.T) {
	t.Parallel()
	handler := api.HandleGetRender(&mockRenderer{renderFn: func(ctx context.Context, url string, printer service.Printer, opts ...service.RenderingOption) ([]byte, string, error) {
		return []byte("hello world"), "text/plain", nil
	}}, config.APIConfig{}, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com&encoding=png", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "text/plain", rec.Header().Get("Content-Type"))
	require.Equal(t, "11", rec.Header().Get("Content-Length"))
	require.Contains(t, rec.Body.String(), "hello world")
}

func TestHandleGetRender_EmptyEncodingUsesDefault(t *testing.T) {
	t.Parallel()
	apiCfg := config.APIConfig{DefaultEncoding: "png"}
	handler := api.HandleGetRender(&mockRenderer{renderFn: func(ctx context.Context, url string, printer service.Printer, opts ...service.RenderingOption) ([]byte, string, error) {
		return []byte("ok"), "image/png", nil
	}}, apiCfg, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com&encoding=", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleGetRender_EncodingWhitespaceTrimmed(t *testing.T) {
	t.Parallel()
	handler := api.HandleGetRender(&mockRenderer{renderFn: func(ctx context.Context, url string, printer service.Printer, opts ...service.RenderingOption) ([]byte, string, error) {
		return []byte("ok"), "text/plain", nil
	}}, config.APIConfig{}, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com&encoding=%20%20%20png%20%20", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleGetRender_MissingRenderParamWithCapture(t *testing.T) {
	t.Parallel()
	engine, err := capture.NewEngine(config.CaptureConfig{SemanticEnabled: true, Timeout: time.Second, MaxJSONBytes: 4096}, testCollector{})
	require.NoError(t, err)
	handler := api.HandleGetRender(&mockRenderer{}, config.APIConfig{}, engine)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com/d-solo/my-uid/my-slug%3FsiamCaptureVersion=1%26siamCaptureKind=test%26panelId=4&encoding=png&renderKey=rk&domain=d", nil))
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleGetRender_HeightValid(t *testing.T) {
	t.Parallel()
	handler := api.HandleGetRender(&mockRenderer{renderFn: func(ctx context.Context, url string, printer service.Printer, opts ...service.RenderingOption) ([]byte, string, error) {
		return []byte("ok"), "text/plain", nil
	}}, config.APIConfig{}, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com&encoding=png&height=500", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleGetRender_WidthAndHeightValid(t *testing.T) {
	t.Parallel()
	handler := api.HandleGetRender(&mockRenderer{renderFn: func(ctx context.Context, url string, printer service.Printer, opts ...service.RenderingOption) ([]byte, string, error) {
		return []byte("ok"), "text/plain", nil
	}}, config.APIConfig{}, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com&encoding=png&width=800&height=600", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleGetRender_PDFPrinterInvalidOption(t *testing.T) {
	t.Parallel()
	engine, err := capture.NewEngine(config.CaptureConfig{SemanticEnabled: false, Timeout: time.Second, MaxJSONBytes: 4096}, testCollector{})
	require.NoError(t, err)
	handler := api.HandleGetRender(&mockRenderer{}, config.APIConfig{}, engine)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/render?url=http://x.com&encoding=pdf&pdf.format=invalid_size", nil))
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/grafana/grafana-image-renderer/pkg/capture"
	"github.com/grafana/grafana-image-renderer/pkg/config"
	"github.com/grafana/grafana-image-renderer/pkg/service"
	"github.com/stretchr/testify/require"
)

const apiCaptureTarget = "http://grafana:3000/d-solo/dash/dashboard?render=1&panelId=4&from=100&to=200&tz=Europe%2FMoscow&siamCaptureVersion=1&siamCaptureKind=grafana-table"

type handlerCollector struct{}

func (handlerCollector) Kind() string { return "grafana-table" }

func (handlerCollector) Collect(context.Context, capture.Request, int) (capture.Collection, error) {
	return capture.Collection{Payload: struct {
		Kind string `json:"kind"`
	}{Kind: "literal"}}, nil
}

type recordingBrowser struct {
	calls       int
	target      string
	printerType string
}

func (b *recordingBrowser) Render(_ context.Context, target string, printer service.Printer, _ ...service.RenderingOption) ([]byte, string, error) {
	b.calls++
	b.target = target
	b.printerType = fmt.Sprintf("%T", printer)
	switch b.printerType {
	case "*service.capturePrinter":
		return []byte(`{"semantic":true}`), "application/json", nil
	case "*service.pdfPrinter":
		return []byte("pdf"), "application/pdf", nil
	default:
		return []byte("png"), "image/png", nil
	}
}

func TestRenderWithoutMarkerUsesOriginalPNGPath(t *testing.T) {
	browser := &recordingBrowser{}
	engine := newAPIEngine(t, true)
	target := "http://grafana:3000/d-solo/dash/dashboard?render=1&panelId=4"

	response := serveRender(t, browser, engine, target, "png")

	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "image/png", response.Header().Get("Content-Type"))
	require.Equal(t, "*service.pngPrinter", browser.printerType)
	require.Equal(t, target, browser.target)
}

func TestSemanticMarkerUsesCapturePrinterAndCleanedTarget(t *testing.T) {
	browser := &recordingBrowser{}
	engine := newAPIEngine(t, true)

	response := serveRender(t, browser, engine, apiCaptureTarget, "png")

	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "application/json", response.Header().Get("Content-Type"))
	require.Equal(t, "*service.capturePrinter", browser.printerType)
	require.NotContains(t, browser.target, "siamCapture")
	require.Contains(t, browser.target, "panelId=4")
}

func TestInvalidSemanticMarkerDoesNotStartBrowser(t *testing.T) {
	browser := &recordingBrowser{}
	engine := newAPIEngine(t, true)
	target := strings.ReplaceAll(apiCaptureTarget, "&siamCaptureKind=grafana-table", "")

	response := serveRender(t, browser, engine, target, "png")

	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Zero(t, browser.calls)
}

func TestDisabledSemanticMarkerDoesNotStartBrowser(t *testing.T) {
	browser := &recordingBrowser{}
	engine := newAPIEngine(t, false)

	response := serveRender(t, browser, engine, apiCaptureTarget, "png")

	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Zero(t, browser.calls)
}

func TestSemanticMarkerRejectsPDFWithoutStartingBrowser(t *testing.T) {
	browser := &recordingBrowser{}
	engine := newAPIEngine(t, true)

	response := serveRender(t, browser, engine, apiCaptureTarget, "pdf")

	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Zero(t, browser.calls)
}

func TestUnmarkedPDFRemainsUnchanged(t *testing.T) {
	browser := &recordingBrowser{}
	engine := newAPIEngine(t, true)
	target := "http://grafana:3000/d-solo/dash/dashboard?render=1&panelId=4"

	response := serveRender(t, browser, engine, target, "pdf")

	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "application/pdf", response.Header().Get("Content-Type"))
	require.Equal(t, "*service.pdfPrinter", browser.printerType)
	require.Equal(t, target, browser.target)
}

func newAPIEngine(t *testing.T, enabled bool) *capture.Engine {
	t.Helper()
	engine, err := capture.NewEngine(config.CaptureConfig{
		SemanticEnabled: enabled,
		Timeout:         time.Second,
		MaxJSONBytes:    4096,
	}, handlerCollector{})
	require.NoError(t, err)
	return engine
}

func serveRender(t *testing.T, browser browserRenderer, engine *capture.Engine, target, encoding string) *httptest.ResponseRecorder {
	t.Helper()
	query := url.Values{
		"url":       {target},
		"encoding":  {encoding},
		"renderKey": {"rk"},
		"domain":    {"grafana"},
		"width":     {"1200"},
		"height":    {"800"},
	}
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/render?"+query.Encode(), nil)
	response := httptest.NewRecorder()
	HandleGetRender(browser, config.APIConfig{DefaultEncoding: "png"}, engine).ServeHTTP(response, request)
	return response
}

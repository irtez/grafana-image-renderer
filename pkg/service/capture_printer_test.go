package service

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"
	"time"

	"github.com/grafana/chromedp"
	"github.com/grafana/grafana-image-renderer/pkg/capture"
	"github.com/grafana/grafana-image-renderer/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestCapturePrinter_NewCapturePrinter(t *testing.T) {
	t.Parallel()
	session := (*capture.Session)(nil)
	printer := NewCapturePrinter(session)
	require.NotNil(t, printer)
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u
}

func validCaptureTransport() capture.Transport {
	return capture.Transport{Encoding: "png", RenderKey: "rk", Domain: "grafana"}
}

func mustCaptureSession(t *testing.T, kind string) *capture.Session {
	t.Helper()
	cfg := config.CaptureConfig{
		SemanticEnabled:  true,
		Timeout:          time.Second,
		MaxJSONBytes:     4096,
		SVGMaxPanels:     1,
		SVGMaxPanelBytes: 1048576,
		SVGMaxJSONBytes:  1048576,
	}
	var collector capture.Collector
	switch kind {
	case "svgmodifier":
		collector = capture.NewSVGCollector()
	case "grafana-table":
		collector = capture.NewTableCollector()
	default:
		collector = capture.NewTableCollector()
	}
	engine, err := capture.NewEngine(cfg, collector)
	require.NoError(t, err)

	var target string
	if kind == "svgmodifier" {
		target = "http://grafana:3000/d/cm-sla/dashboard?render=1&siamCapturePanels=4&from=100&to=200&tz=Europe%2FMoscow&siamCaptureVersion=2&siamCaptureKind=svgmodifier"
	} else {
		target = "http://grafana:3000/d-solo/cm-sla/dashboard?render=1&panelId=4&from=100&to=200&tz=Europe%2FMoscow&var-b=y&var-a=1&var-b=x&siamCaptureVersion=1&siamCaptureKind=grafana-table"
	}
	session, err := engine.Match(mustParseURL(t, target), validCaptureTransport())
	require.NoError(t, err)
	require.NotNil(t, session)
	return session
}

func TestCapturePrinterBeforeNavigateWithNilInitialization(t *testing.T) {
	t.Parallel()
	session := mustCaptureSession(t, "grafana-table")
	printer := NewCapturePrinter(session)
	cp := printer.(*capturePrinter)
	action := cp.beforeNavigate()
	require.Nil(t, action)
}

func TestCapturePrinterBeforeNavigateWithSVG(t *testing.T) {
	t.Parallel()
	session := mustCaptureSession(t, "svgmodifier")
	printer := NewCapturePrinter(session)
	cp := printer.(*capturePrinter)
	action := cp.beforeNavigate()
	require.NotNil(t, action)
}

func TestCapturePrinterUsesProducerReadiness(t *testing.T) {
	t.Parallel()
	session := mustCaptureSession(t, "svgmodifier")
	printer := NewCapturePrinter(session)
	cp := printer.(*capturePrinter)
	require.True(t, cp.usesProducerReadiness())

	session2 := mustCaptureSession(t, "grafana-table")
	printer2 := NewCapturePrinter(session2)
	cp2 := printer2.(*capturePrinter)
	require.False(t, cp2.usesProducerReadiness())
}

func TestCapturePrinterNavigation(t *testing.T) {
	t.Parallel()
	session := mustCaptureSession(t, "grafana-table")
	printer := NewCapturePrinter(session)
	cp := printer.(*capturePrinter)
	var called bool
	action := cp.navigation(chromedp.Tasks{chromedp.ActionFunc(func(context.Context) error {
		called = true
		return nil
	})})
	require.NotNil(t, action)
	require.NoError(t, action.Do(t.Context()))
	require.True(t, called)
}

func TestCapturePrinterPrepareReturnsNilAction(t *testing.T) {
	t.Parallel()
	session := mustCaptureSession(t, "grafana-table")
	printer := NewCapturePrinter(session)
	cp := printer.(*capturePrinter)
	action := cp.prepare(config.BrowserConfig{}, "http://example.com")
	require.NotNil(t, action)
	require.NoError(t, action.Do(t.Context()))
}

func TestCapturePrinterSerializesCollectionFailure(t *testing.T) {
	t.Parallel()
	session := mustCaptureSession(t, "grafana-table")
	printer := NewCapturePrinter(session)
	cp := printer.(*capturePrinter)
	output := make(chan []byte, 1)
	action := cp.action(output, config.BrowserConfig{}, "http://example.com")
	require.NotNil(t, action)
	err := action.Do(t.Context())
	require.NoError(t, err)
	select {
	case body := <-output:
		var envelope struct {
			Contract string `json:"contract"`
			Result   struct {
				Status string `json:"status"`
				Error  struct {
					Code string `json:"code"`
				} `json:"error"`
			} `json:"result"`
		}
		require.NoError(t, json.Unmarshal(body, &envelope))
		require.Equal(t, capture.ContractV1, envelope.Contract)
		require.Equal(t, "error", envelope.Result.Status)
		require.Equal(t, "CAPTURE_INTERNAL_ERROR", envelope.Result.Error.Code)
	default:
		require.Fail(t, "no body sent to output channel")
	}
}

func TestCapturePrinterContentType(t *testing.T) {
	t.Parallel()
	session := mustCaptureSession(t, "grafana-table")
	printer := NewCapturePrinter(session)
	cp := printer.(*capturePrinter)
	require.Equal(t, "application/json", cp.contentType())
}

func TestCapturePrinterActionPropagatesError(t *testing.T) {
	t.Parallel()
	session := mustCaptureSession(t, "svgmodifier")
	printer := NewCapturePrinter(session)
	cp := printer.(*capturePrinter)
	output := make(chan []byte, 1)
	action := cp.action(output, config.BrowserConfig{}, "http://example.com")
	require.NotNil(t, action)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := action.Do(ctx)
	require.Error(t, err)
}

type capturePrinterCollector struct{}

func (capturePrinterCollector) Kind() string { return "grafana-table" }

func (capturePrinterCollector) Collect(context.Context, capture.Request, int) (capture.Collection, error) {
	return capture.Collection{Payload: struct {
		Kind  string `json:"kind"`
		Value string `json:"value"`
	}{Kind: "literal", Value: "ok"}}, nil
}

func TestCapturePrinterReturnsSessionJSONWithoutScreenshot(t *testing.T) {
	engine, err := capture.NewEngine(config.CaptureConfig{
		SemanticEnabled: true,
		Timeout:         time.Second,
		MaxJSONBytes:    4096,
	}, capturePrinterCollector{})
	require.NoError(t, err)
	target, err := url.Parse("http://grafana:3000/d-solo/dash/dashboard?render=1&panelId=4&from=100&to=200&tz=Europe%2FMoscow&siamCaptureVersion=1&siamCaptureKind=grafana-table")
	require.NoError(t, err)
	session, err := engine.Match(target, capture.Transport{Encoding: "png", RenderKey: "rk", Domain: "grafana"})
	require.NoError(t, err)

	printer := NewCapturePrinter(session)
	require.NoError(t, printer.prepare(config.BrowserConfig{}, target.String()).Do(t.Context()))
	output := make(chan []byte, 1)
	require.NoError(t, printer.action(output, config.BrowserConfig{}, target.String()).Do(t.Context()))
	body := <-output

	var envelope struct {
		Contract string `json:"contract"`
		Result   struct {
			Status  string `json:"status"`
			Payload struct {
				Kind  string `json:"kind"`
				Value string `json:"value"`
			} `json:"payload"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	require.Equal(t, capture.ContractV1, envelope.Contract)
	require.Equal(t, "ok", envelope.Result.Status)
	require.Equal(t, "literal", envelope.Result.Payload.Kind)
	require.Equal(t, "ok", envelope.Result.Payload.Value)
	require.Equal(t, "application/json", printer.contentType())
}

package service

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"
	"time"

	"github.com/grafana/grafana-image-renderer/pkg/capture"
	"github.com/grafana/grafana-image-renderer/pkg/config"
	"github.com/stretchr/testify/require"
)

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

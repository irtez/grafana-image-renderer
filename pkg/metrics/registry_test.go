package metrics_test

import (
	"context"
	"net/url"
	"sort"
	"testing"
	"time"

	"github.com/grafana/grafana-image-renderer/pkg/capture"
	"github.com/grafana/grafana-image-renderer/pkg/config"
	"github.com/grafana/grafana-image-renderer/pkg/metrics"
	"github.com/stretchr/testify/require"
)

type semanticMetricCollector struct {
	domainError bool
}

func (semanticMetricCollector) Kind() string { return "grafana-table" }

func (c semanticMetricCollector) Collect(context.Context, capture.Request, int) (capture.Collection, error) {
	if c.domainError {
		return capture.Collection{Error: &capture.Error{Code: "TABLE_QUERY_ERROR", Message: "table query failed"}}, nil
	}
	return capture.Collection{Payload: struct {
		Kind string `json:"kind"`
	}{Kind: "literal"}}, nil
}

func TestNewRegistryWorks(t *testing.T) {
	t.Parallel()

	require.NotPanics(t, func() {
		// We use MustRegister, which can panic. We just want to make sure it doesn't in this case.
		metrics.NewRegistry()
	}, "expected NewRegistry to not panic")
}

func TestSemanticCaptureMetricFamiliesAndLabelsAreBounded(t *testing.T) {
	registry := metrics.NewRegistry()
	runSemanticMetricCapture(t, semanticMetricCollector{})
	runSemanticMetricCapture(t, semanticMetricCollector{domainError: true})

	families, err := registry.Gather()
	require.NoError(t, err)
	wanted := map[string][]string{
		"semantic_capture_requests_total":         {"kind", "result"},
		"semantic_capture_stage_duration_seconds": {"kind", "stage"},
		"semantic_capture_payload_bytes":          {"kind"},
	}
	found := make(map[string]bool, len(wanted))
	for _, family := range families {
		expectedLabels, ok := wanted[family.GetName()]
		if !ok {
			continue
		}
		found[family.GetName()] = true
		for _, metric := range family.Metric {
			labels := make([]string, 0, len(metric.Label))
			for _, label := range metric.Label {
				labels = append(labels, label.GetName())
				require.NotContains(t, label.GetValue(), "dash-secret")
				require.NotContains(t, label.GetValue(), "secret-variable")
			}
			sort.Strings(labels)
			want := append([]string(nil), expectedLabels...)
			sort.Strings(want)
			require.Equal(t, want, labels)
		}
	}
	for name := range wanted {
		require.Truef(t, found[name], "metric family %s is missing", name)
	}
}

func runSemanticMetricCapture(t *testing.T, collector capture.Collector) {
	t.Helper()
	engine, err := capture.NewEngine(config.CaptureConfig{
		SemanticEnabled: true,
		Timeout:         time.Second,
		MaxJSONBytes:    4096,
	}, collector)
	require.NoError(t, err)
	target, err := url.Parse("http://grafana:3000/d-solo/dash-secret/dashboard?render=1&panelId=4&from=100&to=200&tz=Europe%2FMoscow&var-x=secret-variable&siamCaptureVersion=1&siamCaptureKind=grafana-table")
	require.NoError(t, err)
	session, err := engine.Match(target, capture.Transport{Encoding: "png", RenderKey: "rk", Domain: "grafana"})
	require.NoError(t, err)
	_, err = session.Capture(t.Context())
	require.NoError(t, err)
}

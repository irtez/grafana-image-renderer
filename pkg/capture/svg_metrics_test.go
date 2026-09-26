package capture

import (
	"encoding/json"
	"strings"
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

func TestSVGSessionMetricsFollowFinalEnvelope(t *testing.T) {
	ok := func(id int, payload string) PanelResult {
		return PanelResult{PanelID: id, Status: "ok", Payload: json.RawMessage(payload)}
	}
	failed := func(id int) PanelResult {
		return PanelResult{PanelID: id, Status: "error", Error: svgError("CAPTURE_TIMEOUT").Error}
	}
	// Both panels fit individually; only the combined response exceeds the limit.
	large := `{"text":"` + strings.Repeat("x", 2000) + `"}`
	for _, tc := range []struct {
		name      string
		panels    []PanelResult
		global    *Error
		maxBytes  int
		status    string
		errorCode string
		outcome   string
	}{
		{name: "complete", panels: []PanelResult{ok(8, `{}`), ok(7, `{}`)}, status: "complete", outcome: "ok"},
		{name: "partial", panels: []PanelResult{ok(8, `{}`), failed(7)}, status: "partial", outcome: "domain_error"},
		{name: "all panels failed", panels: []PanelResult{failed(8), failed(7)}, status: "failed", outcome: "domain_error"},
		{name: "global failure", global: svgError("CAPTURE_CONTEXT_CHANGED").Error, status: "failed", errorCode: "CAPTURE_CONTEXT_CHANGED", outcome: "domain_error"},
		{name: "total byte limit", panels: []PanelResult{ok(8, large), ok(7, large)}, maxBytes: 3000, status: "failed", errorCode: "CAPTURE_PAYLOAD_TOO_LARGE", outcome: "domain_error"},
		{name: "error envelope cannot fit", panels: []PanelResult{ok(8, `{}`), ok(7, `{}`)}, maxBytes: 10, outcome: "serialize_error"},
		{name: "invalid batch", panels: []PanelResult{ok(8, `{}`)}, outcome: "serialize_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := captureTestConfig(true)
			cfg.SVGMaxPanelBytes = 4096
			cfg.SVGMaxJSONBytes = 10000
			if tc.maxBytes != 0 {
				cfg.SVGMaxJSONBytes = tc.maxBytes
			}
			collector := literalCollector{kind: "svgmodifier", collection: Collection{
				Payload: BatchCollection{Panels: tc.panels}, Error: tc.global,
			}}
			engine, err := NewEngine(cfg, collector)
			require.NoError(t, err)
			session, err := engine.Match(mustTargetURL(t, mapTarget), validTransport())
			require.NoError(t, err)

			count := func(outcome string) float64 {
				metric := &dto.Metric{}
				require.NoError(t, MetricSemanticCaptureRequests.WithLabelValues("svgmodifier", outcome).Write(metric))
				return metric.GetCounter().GetValue()
			}
			before := map[string]float64{}
			for _, outcome := range []string{"ok", "domain_error", "serialize_error", "timeout", "internal_error"} {
				before[outcome] = count(outcome)
			}

			body, err := session.Capture(t.Context())
			if tc.status == "" {
				require.Error(t, err)
				require.Nil(t, body)
			} else {
				require.NoError(t, err)
				var envelope BatchEnvelope
				require.NoError(t, json.Unmarshal(body, &envelope))
				require.Equal(t, tc.status, envelope.Status)
				if tc.errorCode == "" {
					require.Nil(t, envelope.Error)
				} else {
					require.NotNil(t, envelope.Error)
					require.Equal(t, tc.errorCode, envelope.Error.Code)
					require.Len(t, envelope.Panels, 2)
					for _, panel := range envelope.Panels {
						require.Empty(t, panel.Payload)
						require.NotNil(t, panel.Error)
						require.Equal(t, tc.errorCode, panel.Error.Code)
					}
				}
			}
			// One request must increment exactly one outcome, including failures
			// introduced by serialization rather than by the collector.
			for outcome, previous := range before {
				want := float64(0)
				if outcome == tc.outcome {
					want = 1
				}
				require.Equal(t, want, count(outcome)-previous, outcome)
			}
		})
	}
}

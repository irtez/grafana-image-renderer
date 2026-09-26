package capture

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const mapTarget = "http://grafana:3000/d/example/map?render=1&from=now-3h&to=now&tz=Europe%2FMoscow&var-node=b&var-node=a&siamCaptureVersion=2&siamCaptureKind=svgmodifier&siamCapturePanels=8,7"

func TestV2MarkersSelectOrderedBatchOrSolo(t *testing.T) {
	engine, err := NewEngine(captureTestConfig(true), literalCollector{kind: "svgmodifier"})
	require.NoError(t, err)
	for _, raw := range []string{mapTarget, strings.Replace(mapTarget, "/d/", "/grafana/d/", 1)} {
		session, err := engine.Match(mustTargetURL(t, raw), validTransport())
		require.NoError(t, err)
		require.Equal(t, []int{8, 7}, session.Request().PanelIDs)
		require.Equal(t, 2, session.Request().Version)
		cleaned := mustTargetURL(t, session.NavigationURL())
		require.Empty(t, cleaned.Query().Get("siamCapturePanels"))
		require.Equal(t, []string{"b", "a"}, cleaned.Query()["var-node"])
	}
	for _, id := range []string{"7", "panel-7"} {
		raw := strings.Replace(strings.Replace(mapTarget, "/d/", "/d-solo/", 1), "siamCapturePanels=8,7", "panelId="+id, 1)
		s, err := engine.Match(mustTargetURL(t, raw), validTransport())
		require.NoError(t, err)
		require.Equal(t, []int{7}, s.Request().PanelIDs)
	}
}
func TestV2MarkersRejectAmbiguousSelection(t *testing.T) {
	engine, err := NewEngine(captureTestConfig(true), literalCollector{kind: "svgmodifier"})
	require.NoError(t, err)
	for _, value := range []string{"", "7,7", "7,", "-1", "01", "panel-7", "9007199254740992", "7, 8"} {
		_, err := engine.Match(mustTargetURL(t, strings.Replace(mapTarget, "8,7", value, 1)), validTransport())
		require.Error(t, err, value)
	}
	for _, raw := range []string{mapTarget + "&panelId=7", mapTarget + "&siamCapturePanels=8", strings.Replace(mapTarget, "Version=2", "Version=1", 1), strings.Replace(mapTarget, "/d/", "/render/d/", 1), strings.Replace(mapTarget, "/d/", "/d-solo/", 1)} {
		_, err := engine.Match(mustTargetURL(t, raw), validTransport())
		require.Error(t, err)
	}
	cfg := captureTestConfig(true)
	cfg.SVGMaxPanels = 1
	engine, err = NewEngine(cfg, literalCollector{kind: "svgmodifier"})
	require.NoError(t, err)
	_, err = engine.Match(mustTargetURL(t, mapTarget), validTransport())
	require.Error(t, err)
}
func batchRequest() Request {
	return Request{Version: 2, Kind: "svgmodifier", DashboardUID: "example", PanelIDs: []int{8, 7}, RenderFrom: "now-3h", RenderTo: "now", Timezone: "Europe/Moscow", VariablesHash: "sha256:test"}
}
func TestV2EnvelopePreservesOrderStatusAndPrecision(t *testing.T) {
	panels := []PanelResult{{PanelID: 8, Status: "ok", Payload: json.RawMessage(`{"value":12.345678901234567,"name":"éЖ😀"}`)}, {PanelID: 7, Status: "error", Error: &Error{Code: "CAPTURE_TIMEOUT", Message: "timeout"}}}
	body, status, err := MarshalBatch(batchRequest(), panels, nil, 100, 200, 10000)
	require.NoError(t, err)
	require.Equal(t, "partial", status)
	var got BatchEnvelope
	require.NoError(t, json.Unmarshal(body, &got))
	require.Equal(t, ContractV2, got.Contract)
	require.Equal(t, "partial", got.Status)
	require.Equal(t, []int{8, 7}, got.Capture.RequestedPanelIDs)
	require.Equal(t, 8, got.Panels[0].PanelID)
	require.Contains(t, string(got.Panels[0].Payload), "12.345678901234567")
	exact, status, err := MarshalBatch(batchRequest(), panels, nil, 100, 200, len(body))
	require.NoError(t, err)
	require.Equal(t, "partial", status)
	require.Equal(t, body, exact)
	panels[1] = PanelResult{PanelID: 7, Status: "ok", Payload: json.RawMessage(`{}`)}
	body, status, err = MarshalBatch(batchRequest(), panels, nil, 100, 200, 10000)
	require.NoError(t, err)
	require.Equal(t, "complete", status)
	require.NoError(t, json.Unmarshal(body, &got))
	require.Equal(t, "complete", got.Status)
}
func TestV2EnvelopeNeverTruncatesAnOversizedBatch(t *testing.T) {
	panels := []PanelResult{{PanelID: 8, Status: "ok", Payload: json.RawMessage(`{"text":"` + strings.Repeat("é", 2000) + `"}`)}, {PanelID: 7, Status: "ok", Payload: json.RawMessage(`{}`)}}
	body, status, err := MarshalBatch(batchRequest(), panels, nil, 100, 200, 1500)
	require.NoError(t, err)
	require.Equal(t, "failed", status)
	var got BatchEnvelope
	require.NoError(t, json.Unmarshal(body, &got))
	require.Equal(t, "failed", got.Status)
	require.Equal(t, "CAPTURE_PAYLOAD_TOO_LARGE", got.Error.Code)
	require.Len(t, got.Panels, 2)
	for _, p := range got.Panels {
		require.Empty(t, p.Payload)
		require.NotNil(t, p.Error)
	}
	_, status, err = MarshalBatch(batchRequest(), panels, nil, 100, 200, 10)
	require.Error(t, err)
	require.Empty(t, status)
}
func TestV2EnvelopeRejectsMissingDuplicateOrConflictingEntries(t *testing.T) {
	ok := PanelResult{PanelID: 8, Status: "ok", Payload: json.RawMessage(`{}`)}
	for _, rows := range [][]PanelResult{{ok}, {ok, ok}, {ok, {PanelID: 7, Status: "ok"}}, {ok, {PanelID: 7, Status: "ok", Payload: json.RawMessage(`{}`), Error: &Error{Code: "CAPTURE_TIMEOUT"}}}} {
		_, status, err := MarshalBatch(batchRequest(), rows, nil, 100, 200, 10000)
		require.Error(t, err)
		require.Empty(t, status)
	}
}

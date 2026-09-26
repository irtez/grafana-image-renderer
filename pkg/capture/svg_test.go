package capture

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func svgTransportFixture(t *testing.T) (svgScriptResult, Request) {
	t.Helper()
	raw, err := os.ReadFile("testdata/svg-snapshot-v2.json")
	require.NoError(t, err)
	identity := SVGIdentity{ProducerID: "svgmodifier-panel", ProducerVersion: "1.4.0", PanelID: 7, InstanceID: "one"}
	run := SVGRun{Generation: 1, EffectiveFromMs: 0, EffectiveToMs: 1000}
	body := string(raw)
	return svgScriptResult{Status: "terminal-ok", Identity: &identity, Run: &run, SnapshotJSON: &body, PayloadBytes: len(raw)},
		Request{PanelID: 7, Kind: "svgmodifier"}
}

func TestSVGCollectorKeepsCompletePayloadWithoutTrimming(t *testing.T) {
	state, request := svgTransportFixture(t)
	collection, terminal := svgCollectionFromScript(state, request, 1024*1024)
	require.True(t, terminal)
	require.Nil(t, collection.Error)
	raw, ok := collection.Payload.(json.RawMessage)
	require.True(t, ok)
	require.Equal(t, *state.SnapshotJSON, string(raw))
	_, trims := collection.Payload.(PayloadTrimmer)
	require.False(t, trims)
	body, status, err := MarshalBatch(Request{PanelIDs: []int{7}}, []PanelResult{{PanelID: 7, Status: "ok", Payload: raw}}, nil, 0, 1, 1024)
	require.NoError(t, err)
	require.Equal(t, "failed", status)
	require.Contains(t, string(body), `"code":"CAPTURE_PAYLOAD_TOO_LARGE"`)
}

func TestSVGCollectorWaitsWithoutReturningOldPayload(t *testing.T) {
	for _, status := range []string{"idle", "pending"} {
		state := svgScriptResult{Status: status}
		collection, terminal := svgCollectionFromScript(state, Request{PanelID: 7}, 1024)
		require.False(t, terminal)
		require.Nil(t, collection.Payload)
		old := `{}`
		state.SnapshotJSON = &old
		collection, terminal = svgCollectionFromScript(state, Request{PanelID: 7}, 1024)
		require.True(t, terminal)
		require.Equal(t, "CAPTURE_PAYLOAD_INVALID", collection.Error.Code)
	}
}

func TestSVGCollectorRejectsMalformedOrOversizedState(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*svgScriptResult)
		limit  int
		code   string
	}{
		{"missing identity", func(s *svgScriptResult) { s.Identity = nil }, 1048576, "CAPTURE_PAYLOAD_INVALID"},
		{"wrong panel", func(s *svgScriptResult) { s.Identity.PanelID++ }, 1048576, "CAPTURE_PAYLOAD_INVALID"},
		{"wrong generation", func(s *svgScriptResult) { s.Run.Generation++ }, 1048576, "CAPTURE_PAYLOAD_INVALID"},
		{"wrong time", func(s *svgScriptResult) { s.Run.EffectiveToMs++ }, 1048576, "CAPTURE_PAYLOAD_INVALID"},
		{"missing run", func(s *svgScriptResult) { s.Run = nil }, 1048576, "CAPTURE_PAYLOAD_INVALID"},
		{"missing payload", func(s *svgScriptResult) { s.SnapshotJSON = nil }, 1048576, "CAPTURE_PAYLOAD_INVALID"},
		{"wrong bytes", func(s *svgScriptResult) { s.PayloadBytes++ }, 1048576, "CAPTURE_PAYLOAD_INVALID"},
		{"unknown status", func(s *svgScriptResult) { s.Status = "success" }, 1048576, "CAPTURE_PAYLOAD_INVALID"},
		{"payload with error", func(s *svgScriptResult) { s.Error = &svgScriptError{Code: "CAPTURE_EXPORT_FAILED"} }, 1048576, "CAPTURE_PAYLOAD_INVALID"},
		{"too large", func(*svgScriptResult) {}, 100, "CAPTURE_PAYLOAD_TOO_LARGE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state, request := svgTransportFixture(t)
			tc.change(&state)
			collection, terminal := svgCollectionFromScript(state, request, tc.limit)
			require.True(t, terminal)
			require.Nil(t, collection.Payload)
			require.Equal(t, tc.code, collection.Error.Code)
		})
	}
}

func TestSVGCollectorAllowsOnlySafeErrorCodes(t *testing.T) {
	for _, code := range []string{"CAPTURE_PRODUCER_MISSING", "CAPTURE_PROTOCOL_UNSUPPORTED", "CAPTURE_INSTANCE_AMBIGUOUS", "CAPTURE_FRAME_UNSUPPORTED", "CAPTURE_DATA_STATE_UNSUPPORTED", "CAPTURE_EXPORT_FAILED", "CAPTURE_PAYLOAD_INVALID", "CAPTURE_PAYLOAD_TOO_LARGE", "private-value"} {
		collection, terminal := svgCollectionFromScript(svgScriptResult{Status: "terminal-error", Error: &svgScriptError{Code: code}}, Request{}, 1024)
		require.True(t, terminal)
		require.Nil(t, collection.Payload)
		if code == "private-value" {
			require.Equal(t, "CAPTURE_PAYLOAD_INVALID", collection.Error.Code)
		} else {
			require.Equal(t, code, collection.Error.Code)
		}
		require.NotContains(t, collection.Error.Message, "private-value")
	}
}

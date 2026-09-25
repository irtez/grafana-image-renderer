package capture

import (
	"bytes"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"os"
	"strconv"
	"strings"
	"testing"
)

func svgValidationFixture(t *testing.T, name string) json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)
	return raw
}
func svgValidationContext() (Request, SVGIdentity, SVGRun) {
	return Request{Version: 2, Kind: "svgmodifier", PanelID: 7}, SVGIdentity{ProducerID: "svgmodifier-panel", ProducerVersion: "1.4.0", PanelID: 7, InstanceID: "example"}, SVGRun{Generation: 1, EffectiveFromMs: 0, EffectiveToMs: 1000}
}
func decodeSVGFixture(t *testing.T, raw []byte) any {
	t.Helper()
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var v any
	require.NoError(t, d.Decode(&v))
	return v
}
func svgFixtureAt(v any, path string) any {
	for _, part := range strings.Split(path, "/") {
		switch n := v.(type) {
		case map[string]any:
			v = n[part]
		case []any:
			i, _ := strconv.Atoi(part)
			v = n[i]
		}
	}
	return v
}
func svgFixtureSet(v any, path string, replacement any) {
	at := strings.LastIndex(path, "/")
	key := path
	if at >= 0 {
		v = svgFixtureAt(v, path[:at])
		key = path[at+1:]
	}
	switch n := v.(type) {
	case map[string]any:
		n[key] = replacement
	case []any:
		i, _ := strconv.Atoi(key)
		n[i] = replacement
	}
}
func TestValidateV2SnapshotPreservesProducerPayload(t *testing.T) {
	raw := svgValidationFixture(t, "svg-snapshot-v2.json")
	before := bytes.Clone(raw)
	request, identity, run := svgValidationContext()
	require.NoError(t, validateSVGSnapshot(raw, request, identity, run))
	require.Equal(t, before, []byte(raw))
}
func TestValidateV2SnapshotRejectsSchemaReferencesAndStates(t *testing.T) {
	for _, tc := range []struct {
		path  string
		value any
	}{
		{"schemaVersion", 1}, {"diagram", map[string]any{}}, {"producer/id", "other"}, {"observed/generation", 0}, {"observed/effectiveFromMs", 1001}, {"observed/dataState", "Loading"},
		{"metrics/0/settings", map[string]any{}}, {"metrics/0/points", []any{}}, {"metrics/0/scalar/value", "private-value"}, {"metrics/0/scalar", nil}, {"metrics/0/availability", "unavailable"},
		{"metrics/0/ruleId", "missing"}, {"metrics/0/indicatorIds", []string{}}, {"metrics/1/id", "metric-a"},
		{"objects/0/indicatorIds", []string{"missing"}}, {"objects/0/parentId", "object-a"}, {"objects/0/parentRelation", "inferred"},
		{"indicators/0/objectIds", []string{"missing"}}, {"indicators/0/binding/status", "ambiguous"}, {"indicators/0/metricIds", []string{"missing"}},
		{"indicators/0/state/winnerMetricId", "metric-missing"}, {"indicators/0/state/winnerRowIndex", 0}, {"indicators/0/state/noData", true}, {"indicators/0/state/selectedRuleId", nil},
		{"indicators/0/ruleResults/0/winnerMetricId", nil}, {"indicators/0/navigation/0/linkId", "missing"}, {"indicators/0/tooltip/metricIds", []string{"missing"}},
		{"indicators/0/appearance/0/fill/rgba/3", 2}, {"indicators/0/appearance/0/fill/kind", "none"}, {"metrics/0/scalar/appliedThreshold/index", -1},
		{"diagnostics/0/causeIds", []string{"missing"}}, {"diagnostics/0/metricIds", []string{"unknown"}}, {"diagnostics/0/ruleIds", []string{"unknown"}},
	} {
		t.Run(tc.path, func(t *testing.T) {
			value := decodeSVGFixture(t, svgValidationFixture(t, "svg-snapshot-v2.json"))
			svgFixtureSet(value, tc.path, tc.value)
			raw, err := json.Marshal(value)
			require.NoError(t, err)
			request, identity, run := svgValidationContext()
			err = validateSVGSnapshot(raw, request, identity, run)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "private-value")
		})
	}
}
func TestValidateV2SnapshotRejectsMalformedJSONAndBoundedWork(t *testing.T) {
	raw := svgValidationFixture(t, "svg-snapshot-v2.json")
	request, identity, run := svgValidationContext()
	for _, bad := range [][]byte{append(bytes.Clone(raw), []byte(`{}`)...), bytes.Replace(raw, []byte(`"kind": "svgmodifier"`), []byte(`"kind":"wrong","kind":"svgmodifier"`), 1), bytes.Replace(raw, []byte("Example"), []byte{0xff}, 1), bytes.Replace(raw, []byte("12.34567"), []byte("0e4097"), 1), []byte(strings.Repeat("[", 100) + "0" + strings.Repeat("]", 100))} {
		require.Error(t, validateSVGSnapshot(bad, request, identity, run))
	}
}
func v2Table(t *testing.T) any {
	v := decodeSVGFixture(t, svgValidationFixture(t, "svg-snapshot-v2.json"))
	decision := svgFixtureAt(v, "metrics/0/scalar")
	table := map[string]any{"columns": []any{map[string]any{"name": "Node", "type": "string"}, map[string]any{"name": "Value", "type": "number"}}, "headers": []string{"Node", "Value"}, "rowFilterStatus": "applied", "thresholdColumnIndex": 1, "winningRowIndex": 0,
		"rows": []any{map[string]any{"sourceIndex": 5, "values": []any{"same", 12.34567}, "displayValues": []any{"same", "12.35"}, "decision": decision, "cellIssues": []any{}}, map[string]any{"sourceIndex": 6, "values": []any{"same", nil}, "displayValues": []any{"same", nil}, "decision": nil, "cellIssues": []any{}}}}
	for path, value := range map[string]any{"metrics/0/kind": "table", "metrics/0/scalar": nil, "metrics/0/table": table, "indicators/0/state/winnerRowIndex": 0, "indicators/0/ruleResults/0/winnerRowIndex": 0, "indicators/0/tooltip/metricIds": []any{}, "indicators/0/tooltip/tables": []any{map[string]any{"metricId": "metric-a", "rowIndices": []int{1, 0}}}} {
		svgFixtureSet(v, path, value)
	}
	return v
}
func TestValidateV2TableKeepsSourceIndicesAndTooltipOrder(t *testing.T) {
	request, identity, run := svgValidationContext()
	value := v2Table(t)
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	require.NoError(t, validateSVGSnapshot(raw, request, identity, run))
	for _, tc := range []struct {
		path  string
		value any
	}{{"metrics/0/table/winningRowIndex", 1}, {"metrics/0/table/rows/0/values", []any{}}, {"metrics/0/table/rows/1/sourceIndex", 5}, {"metrics/0/table/thresholdColumnIndex", 2}, {"indicators/0/tooltip/tables/0/rowIndices", []int{2}}, {"metrics/0/table/rows/0/decision", nil}} {
		v := decodeSVGFixture(t, raw)
		svgFixtureSet(v, tc.path, tc.value)
		bad, err := json.Marshal(v)
		require.NoError(t, err)
		require.Error(t, validateSVGSnapshot(bad, request, identity, run), tc.path)
	}
}
func TestValidateV2SnapshotConcurrentCalls(t *testing.T) {
	request, identity, run := svgValidationContext()
	raw := svgValidationFixture(t, "svg-snapshot-v2.json")
	done := make(chan error, 16)
	for range 16 {
		go func() { done <- validateSVGSnapshot(raw, request, identity, run) }()
	}
	for range 16 {
		require.NoError(t, <-done)
	}
}

func TestValidateV2AcceptsLargeJSONWithinByteBudget(t *testing.T) {
	request, identity, run := svgValidationContext()
	v := v2Table(t)
	svgFixtureSet(v, "metrics/0/table/rows/1/values", []any{make([]int, 120000), nil})
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	require.Less(t, len(raw), 4194304)
	require.NoError(t, validateSVGSnapshot(raw, request, identity, run))
}
func TestValidateV2SafetyLimitHasDistinctError(t *testing.T) {
	request, identity, run := svgValidationContext()
	require.ErrorIs(t, validateSVGSnapshot([]byte(strings.Repeat("[", 65)+"0"+strings.Repeat("]", 65)), request, identity, run), errSVGValidationLimit)
	raw := svgValidationFixture(t, "svg-snapshot-v2.json")
	raw = bytes.Replace(raw, []byte("12.34567"), []byte("0e4097"), 1)
	require.ErrorIs(t, validateSVGSnapshot(raw, request, identity, run), errSVGValidationLimit)
}

package capture

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func svgValidationFixture(t *testing.T, name string) json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)
	return raw
}

func svgValidationContext() (Request, SVGIdentity, SVGRun) {
	return Request{Kind: "svgmodifier", PanelID: 7, RenderFrom: "100", RenderTo: "200"},
		SVGIdentity{ProducerID: "svgmodifier-panel", ProducerVersion: "1.4.0", PanelID: 7, InstanceID: "example-instance"},
		SVGRun{Generation: 3, EffectiveFromMs: 1700000000000, EffectiveToMs: 1700003600000}
}

func decodeSVGFixture(t *testing.T, raw []byte) any {
	t.Helper()
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	require.NoError(t, decoder.Decode(&value))
	return value
}

func svgFixtureAt(value any, path string) any {
	for _, key := range strings.Split(path, "/") {
		switch node := value.(type) {
		case map[string]any:
			value = node[key]
		case []any:
			i, _ := strconv.Atoi(key)
			value = node[i]
		}
	}
	return value
}

func svgFixtureSet(value any, path string, replacement any) {
	key := path
	if last := strings.LastIndex(path, "/"); last >= 0 {
		key = path[last+1:]
		value = svgFixtureAt(value, path[:last])
	}
	switch node := value.(type) {
	case map[string]any:
		node[key] = replacement
	case []any:
		i, _ := strconv.Atoi(key)
		node[i] = replacement
	}
}

func TestSVGSchemaFingerprint(t *testing.T) {
	raw, err := os.ReadFile("svgmodifier-snapshot-v1.schema.json")
	require.NoError(t, err)
	digest := sha256.Sum256(raw)
	require.Equal(t, "dbc2586c81a93aa30097740c2070f2fc484699690fb4764ad28182a1ba3506b5", hex.EncodeToString(digest[:]))
}

func TestValidateSVGSnapshotAcceptsCompleteExamplesAndPanelOverrides(t *testing.T) {
	request, identity, run := svgValidationContext()
	for _, name := range []string{"svg-snapshot-v1.json", "svg-invalid-configuration-v1.json"} {
		t.Run(name, func(t *testing.T) {
			raw := svgValidationFixture(t, name)
			before := bytes.Clone(raw)
			require.NoError(t, validateSVGSnapshot(raw, request, identity, run))
			require.Equal(t, before, []byte(raw))
		})
	}
}

func TestValidateSVGSnapshotRejectsSchemaViolations(t *testing.T) {
	for _, tc := range []struct {
		path  string
		value any
	}{
		{"kind", "table"}, {"schemaVersion", 2}, {"producer/id", "other"},
		{"producer/version", ""}, {"producer/extra", "private-value"}, {"panel/id", -1},
		{"panel/id", json.Number("9007199254740992")}, {"panel/mode", "other"}, {"panel/title", 1},
		{"observed/generation", 0}, {"observed/dataState", "Loading"},
		{"observed/effectiveFromMs", 1.5}, {"observed/evaluatedAtMs", json.Number("9007199254740992")},
		{"evaluationStatus", "ok"}, {"configuration/yamlStatus", "other"},
		{"configuration/rules", nil}, {"configuration/rules/0/source/line", 0},
		{"configuration/rules/0/source/pageIndex", -1}, {"configuration/rules/0/source/extra", true},
		{"configuration/rules/0/authoredAttributes", []any{}},
		{"configuration/rules/0/elementIds", []string{"cell-alpha", "cell-alpha"}},
		{"elements/0/id", ""}, {"elements/0/ruleResults/0/extra", true}, {"elements/0/noData", map[string]any{}},
		{"metrics/0/queryCounter", 0}, {"metrics/0/selection", "other"}, {"metrics/0/selectors/extra", true},
		{"metrics/0/sources/0/calculation", "mean"}, {"metrics/0/sources/0/labels/node", 7},
		{"metrics/0/sources/0/dataSource/extra", true}, {"metrics/0/settings", []any{}},
		{"metrics/0/availability", "partial"}, {"metrics/0/kind", "number"},
		{"metrics/0/scalar/value", "12"}, {"metrics/0/scalar/extra", true},
		{"metrics/0/scalar/thresholdTrace/0/comparison", "unknown"},
		{"metrics/2/table/rows/0/sourceIndex", -1}, {"metrics/2/table/columns/0/extra", true},
		{"metrics/2/table/rows/0/displayValues/0", false}, {"metrics/2/table/rowFilterStatus", "other"},
		{"expressions/0/inputs/0/calculation", "mean"}, {"expressions/0/extra", true},
		{"diagnostics/0/severity", "info"}, {"diagram/status", "ok"}, {"diagram/viewport/width", -1},
		{"diagram/items/1/paints/0/fill/rgba", []int{1, 2, 3}},
		{"diagram/items/1/paints/0/fill/rgba/0", 256}, {"diagram/items/1/paints/0/fill/rgba/3", 1.1},
		{"diagram/items/1/paints/0/opacity", -0.1}, {"diagram/items/1/paints/0/markers/extra", true},
		{"diagram/items/1/paints/0/nodePath/0", -1}, {"diagram/items/1/links/declarations/0/origin", "other"},
		{"diagram/connections/0/origin", "inferred"}, {"diagram/connections/0/source/extra", true},
		{"extra", "private-value"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			value := decodeSVGFixture(t, svgValidationFixture(t, "svg-snapshot-v1.json"))
			svgFixtureSet(value, tc.path, tc.value)
			raw, err := json.Marshal(value)
			require.NoError(t, err)
			request, identity, run := svgValidationContext()
			err = validateSVGSnapshot(raw, request, identity, run)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "private-value")
		})
	}
	t.Run("missing required nullable field", func(t *testing.T) {
		value := decodeSVGFixture(t, svgValidationFixture(t, "svg-snapshot-v1.json"))
		delete(svgFixtureAt(value, "panel").(map[string]any), "title")
		raw, err := json.Marshal(value)
		require.NoError(t, err)
		request, identity, run := svgValidationContext()
		require.Error(t, validateSVGSnapshot(raw, request, identity, run))
	})
}

func TestValidateSVGSnapshotRejectsBrokenReferencesAndStates(t *testing.T) {
	for _, tc := range []struct {
		path  string
		value any
	}{
		{"configuration/diagnosticIds", []string{"missing"}}, {"configuration/rules/0/elementIds", []string{"missing"}},
		{"configuration/rules/0/elementIds", []string{"cell-table"}}, {"configuration/rules/0/diagnosticIds", []string{"missing"}},
		{"metrics/0/ruleId", "missing"}, {"metrics/0/ruleId", "rule-table"}, {"metrics/0/elementIds", []string{"missing"}},
		{"metrics/0/elementIds", []string{"cell-table"}}, {"metrics/0/metricsIndex", 1}, {"metrics/0/queryIndex", 2},
		{"metrics/0/sources/0/fromMs", json.Number("1700003600001")},
		{"metrics/0/sources/0/diagnosticIds", []string{"missing"}}, {"metrics/0/diagnosticIds", []string{"missing"}},
		{"metrics/0/availability", "unavailable"}, {"metrics/0/scalar", nil}, {"metrics/0/kind", "table"},
		{"metrics/1/availability", "available"}, {"metrics/2/availability", "unavailable"}, {"metrics/2/table", nil},
		{"metrics/0/scalar/thresholdTrace/0/index", 1}, {"metrics/0/settings/thresholds", "wrong-type"},
		{"metrics/0/scalar/thresholdTrace/0/matched", false}, {"metrics/0/scalar/thresholdTrace/0/condition", "false"},
		{"metrics/0/scalar/thresholdTrace/0/diagnosticIds", []string{"missing"}},
		{"metrics/0/scalar/selectedThresholdIndex", nil}, {"metrics/2/table/thresholdColumnIndex", 3},
		{"metrics/2/table/thresholdColumnIndex", nil}, {"metrics/2/table/winningRowIndex", 2},
		{"metrics/2/table/rows/0/decision", nil}, {"metrics/2/table/rows/1/sourceIndex", 3},
		{"metrics/2/table/rows/0/values", []any{"worker-a"}}, {"metrics/2/table/rows/0/displayValues", []any{}},
		{"metrics/2/table/rows/0/cellIssues", []any{map[string]any{"columnIndex": 3, "diagnosticIds": []string{"missing-input"}}}},
		{"metrics/2/table/rows/1/cellIssues", []any{map[string]any{"columnIndex": 2, "diagnosticIds": []string{}}}},
		{"metrics/2/table/rows/1/cellIssues", []any{map[string]any{"columnIndex": 1, "diagnosticIds": []string{"missing-input"}}}},
		{"metrics/2/table/rows/1/cellIssues", []any{map[string]any{"columnIndex": 2, "diagnosticIds": []string{"missing"}}}},
		{"elements/0/diagramItemId", "missing"}, {"elements/0/diagramItemId", "table"},
		{"elements/0/ruleResults/0/ruleId", "missing"}, {"elements/0/ruleResults/0/metricIds", []string{"missing"}},
		{"elements/0/ruleResults/0/metricIds", []string{"metric-table"}},
		{"elements/0/ruleResults/0/winnerMetricId", "metric-missing"},
		{"elements/0/ruleResults/0/winnerRowIndex", 0}, {"elements/0/winnerRowIndex", 0},
		{"elements/0/selectedRuleId", "missing"}, {"elements/0/selectedRuleId", nil},
		{"elements/0/winnerMetricId", "metric-missing"}, {"elements/0/winnerMetricId", nil},
		{"elements/0/noData", map[string]any{"filling": "none"}}, {"elements/0/diagnosticIds", []string{"missing"}},
		{"elements/1/ruleResults/0/winnerRowIndex", nil}, {"elements/1/winnerRowIndex", 1},
		{"expressions/0/value", nil}, {"expressions/0/inputs/0/value", nil},
		{"expressions/0/diagnosticIds", []string{"missing"}}, {"expressions/0/inputs/0/diagnosticIds", []string{"missing"}},
		{"diagnostics/0/ruleIds", []string{"missing"}}, {"diagnostics/0/elementIds", []string{"missing"}},
		{"diagnostics/0/metricIds", []string{"missing"}}, {"diagram/diagnosticIds", []string{"missing"}},
		{"diagram/viewport", nil}, {"diagram/coordinateSpace", nil}, {"diagram/status", "not_rendered"},
		{"diagram/items/0/parentId", "missing"}, {"diagram/items/0/parentId", "drawing"},
		{"diagram/items/0/parentId", "alpha"}, {"diagram/items/1/diagnosticIds", []string{"missing"}},
		{"diagram/items/1/links/declarations/0/ruleId", nil},
		{"diagram/items/1/links/declarations/0/ruleId", "missing"},
		{"diagram/items/1/links/declarations/0/origin", "svg"},
		{"diagram/connections/0/itemId", "missing"}, {"diagram/connections/0/source/itemId", "missing"},
		{"diagram/connections/0/source/cellId", "other"}, {"diagram/connections/0/source/svgId", "other"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			value := decodeSVGFixture(t, svgValidationFixture(t, "svg-snapshot-v1.json"))
			svgFixtureSet(value, tc.path, tc.value)
			raw, err := json.Marshal(value)
			require.NoError(t, err)
			request, identity, run := svgValidationContext()
			require.Error(t, validateSVGSnapshot(raw, request, identity, run))
		})
	}
}

func TestValidateSVGSnapshotRejectsDuplicateIDs(t *testing.T) {
	for _, path := range []string{"configuration/rules", "elements", "metrics", "expressions", "diagnostics", "diagram/items", "diagram/connections", "elements/0/ruleResults", "metrics/0/scalar/thresholdTrace"} {
		t.Run(path, func(t *testing.T) {
			value := decodeSVGFixture(t, svgValidationFixture(t, "svg-snapshot-v1.json"))
			items := svgFixtureAt(value, path).([]any)
			svgFixtureSet(value, path, append(items, items[0]))
			raw, err := json.Marshal(value)
			require.NoError(t, err)
			request, identity, run := svgValidationContext()
			require.Error(t, validateSVGSnapshot(raw, request, identity, run))
		})
	}
}

func TestValidateSVGSnapshotMatchesReceiverIdentityAndRun(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Request, *SVGIdentity, *SVGRun)
	}{
		{"requested panel", func(r *Request, _ *SVGIdentity, _ *SVGRun) { r.PanelID = 8 }},
		{"requested kind", func(r *Request, _ *SVGIdentity, _ *SVGRun) { r.Kind = "table" }},
		{"producer", func(_ *Request, i *SVGIdentity, _ *SVGRun) { i.ProducerID = "other" }},
		{"producer version", func(_ *Request, i *SVGIdentity, _ *SVGRun) { i.ProducerVersion = "1.4.1" }},
		{"identity panel", func(_ *Request, i *SVGIdentity, _ *SVGRun) { i.PanelID = 8 }},
		{"missing instance", func(_ *Request, i *SVGIdentity, _ *SVGRun) { i.InstanceID = "" }},
		{"generation", func(_ *Request, _ *SVGIdentity, r *SVGRun) { r.Generation++ }},
		{"from", func(_ *Request, _ *SVGIdentity, r *SVGRun) { r.EffectiveFromMs++ }},
		{"to", func(_ *Request, _ *SVGIdentity, r *SVGRun) { r.EffectiveToMs++ }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request, identity, run := svgValidationContext()
			tc.mutate(&request, &identity, &run)
			require.Error(t, validateSVGSnapshot(svgValidationFixture(t, "svg-snapshot-v1.json"), request, identity, run))
		})
	}
	t.Run("reversed observed time also matches run", func(t *testing.T) {
		request, identity, run := svgValidationContext()
		value := decodeSVGFixture(t, svgValidationFixture(t, "svg-snapshot-v1.json"))
		run.EffectiveFromMs = run.EffectiveToMs + 1
		svgFixtureSet(value, "observed/effectiveFromMs", run.EffectiveFromMs)
		raw, err := json.Marshal(value)
		require.NoError(t, err)
		require.Error(t, validateSVGSnapshot(raw, request, identity, run))
	})
}

func TestValidateSVGSnapshotPreservesJSONValuesThroughEnvelope(t *testing.T) {
	value := decodeSVGFixture(t, svgValidationFixture(t, "svg-snapshot-v1.json"))
	svgFixtureSet(value, "configuration/rules/0/authoredAttributes/arbitrary", map[string]any{
		"unknownSetting": []any{false, nil, "<svg>Пример</svg>", json.Number("9007199254740991"), json.Number("1.234567890123456789e-12")},
	})
	svgFixtureSet(value, "metrics/0/settings/unknownSetting", map[string]any{"threshold": "invalid declaration", "negativeZero": json.Number("-0")})
	svgFixtureSet(value, "metrics/0/scalar/value", json.Number("1.234567890123456789e-12"))
	svgFixtureSet(value, "metrics/0/scalar/color", "uninterpreted-color")
	svgFixtureSet(value, "metrics/0/scalar/level", json.Number("19"))
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	before := bytes.Clone(raw)
	request, identity, run := svgValidationContext()
	require.NoError(t, validateSVGSnapshot(raw, request, identity, run))
	require.Equal(t, before, raw)

	meta := Metadata{Kind: "svgmodifier", PanelID: 7}
	body, err := MarshalCollection(meta, Collection{Payload: json.RawMessage(raw)}, 1<<20)
	require.NoError(t, err)
	var envelope struct {
		Result struct {
			Status  string
			Payload json.RawMessage
			Error   *Error
		}
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	require.Equal(t, "ok", envelope.Result.Status)
	require.Equal(t, value, decodeSVGFixture(t, envelope.Result.Payload))
	require.Contains(t, string(body), "1.234567890123456789e-12")
	require.Contains(t, string(body), `"negativeZero":-0`)
	_, trimmer := any(json.RawMessage(raw)).(PayloadTrimmer)
	require.False(t, trimmer)

	exact, err := MarshalCollection(meta, Collection{Payload: json.RawMessage(raw)}, len(body))
	require.NoError(t, err)
	require.Equal(t, body, exact)
	tooLarge, err := MarshalCollection(meta, Collection{Payload: json.RawMessage(raw)}, len(body)-1)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(tooLarge, &envelope))
	require.Equal(t, "error", envelope.Result.Status)
	require.Equal(t, "CAPTURE_PAYLOAD_TOO_LARGE", envelope.Result.Error.Code)
	require.NotContains(t, string(tooLarge), `"payload"`)
	require.Equal(t, before, raw)
}

func TestValidateSVGSnapshotRejectsInvalidJSONAndBoundedWork(t *testing.T) {
	raw := svgValidationFixture(t, "svg-snapshot-v1.json")
	request, identity, run := svgValidationContext()
	for _, invalid := range []string{
		"null", "[]", "{}", string(raw) + "{}", string(raw[:len(raw)-3]),
		strings.Replace(string(raw), "12.3456789012", "NaN", 1),
		strings.Replace(string(raw), "12.3456789012", "1e9999", 1),
	} {
		require.Error(t, validateSVGSnapshot(json.RawMessage(invalid), request, identity, run))
	}
	for _, nested := range []string{strings.Repeat("[", 65) + "0" + strings.Repeat("]", 65), "[" + strings.Repeat("0,", 100000) + "0]"} {
		value := decodeSVGFixture(t, raw)
		svgFixtureSet(value, "configuration/rules/0/authoredAttributes/arbitrary", json.RawMessage(nested))
		oversized, err := json.Marshal(value)
		require.NoError(t, err)
		require.Error(t, validateSVGSnapshot(oversized, request, identity, run))
	}
}

func TestValidateSVGSnapshotAcceptsPartialStatesWithoutRecomputation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(any)
	}{
		{"unavailable scalar", func(value any) {
			svgFixtureSet(value, "metrics/1/kind", "scalar")
		}},
		{"table with retained rows and no winner", func(value any) {
			svgFixtureSet(value, "metrics/2/availability", "unavailable")
			svgFixtureSet(value, "metrics/2/table/winningRowIndex", nil)
			svgFixtureSet(value, "elements/1/winnerMetricId", nil)
			svgFixtureSet(value, "elements/1/winnerRowIndex", nil)
			svgFixtureSet(value, "elements/1/ruleResults/0/winnerMetricId", nil)
			svgFixtureSet(value, "elements/1/ruleResults/0/winnerRowIndex", nil)
			svgFixtureSet(value, "elements/1/noData", map[string]any{"filling": "none"})
		}},
		{"table without threshold column", func(value any) {
			svgFixtureSet(value, "metrics/2/table/thresholdColumnIndex", nil)
			svgFixtureSet(value, "metrics/2/table/winningRowIndex", nil)
			for _, row := range svgFixtureAt(value, "metrics/2/table/rows").([]any) {
				row.(map[string]any)["decision"] = nil
			}
			svgFixtureSet(value, "elements/1/winnerMetricId", nil)
			svgFixtureSet(value, "elements/1/winnerRowIndex", nil)
			svgFixtureSet(value, "elements/1/ruleResults/0/winnerMetricId", nil)
			svgFixtureSet(value, "elements/1/ruleResults/0/winnerRowIndex", nil)
		}},
		{"diagnosed null cell", func(value any) {
			svgFixtureSet(value, "metrics/2/table/rows/1/cellIssues", []any{map[string]any{"columnIndex": 2, "diagnosticIds": []string{"missing-input"}}})
		}},
		{"authored invalid metrics declaration", func(value any) {
			svgFixtureSet(value, "configuration/rules/0/attributes/metrics", "invalid but retained")
		}},
		{"authored invalid query declaration", func(value any) {
			svgFixtureSet(value, "configuration/rules/0/attributes/metrics/0/queries", false)
		}},
		{"integer scientific notation", func(value any) {
			svgFixtureSet(value, "panel/id", json.Number("7e0"))
			svgFixtureSet(value, "observed/generation", json.Number("3.00"))
		}},
		{"error terminal state", func(value any) {
			svgFixtureSet(value, "observed/dataState", "Error")
		}},
		{"unavailable expression input", func(value any) {
			svgFixtureSet(value, "expressions/0/inputs/0/value", nil)
			svgFixtureSet(value, "expressions/0/inputs/0/availability", "unavailable")
		}},
		{"finite numeric limits", func(value any) {
			svgFixtureSet(value, "metrics/0/settings/numbers", []any{json.Number("1.7976931348623157e308"), json.Number("5e-324"), json.Number("-0")})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := decodeSVGFixture(t, svgValidationFixture(t, "svg-snapshot-v1.json"))
			tc.mutate(value)
			raw, err := json.Marshal(value)
			require.NoError(t, err)
			request, identity, run := svgValidationContext()
			require.NoError(t, validateSVGSnapshot(raw, request, identity, run))
		})
	}
}

func TestValidateSVGSnapshotChecksUnrenderedDiagramFacts(t *testing.T) {
	value := decodeSVGFixture(t, svgValidationFixture(t, "svg-snapshot-v1.json"))
	svgFixtureSet(value, "diagram/status", "not_rendered")
	svgFixtureSet(value, "diagram/viewport", nil)
	svgFixtureSet(value, "diagram/coordinateSpace", nil)
	for _, value := range svgFixtureAt(value, "diagram/items").([]any) {
		item := value.(map[string]any)
		item["bounds"] = nil
		item["paints"] = []any{}
		item["visibility"] = map[string]any{"display": nil, "visibility": nil, "opacity": nil}
		for _, value := range item["textFragments"].([]any) {
			value.(map[string]any)["bounds"] = nil
		}
	}
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	request, identity, run := svgValidationContext()
	require.NoError(t, validateSVGSnapshot(raw, request, identity, run))
	for _, tc := range []struct {
		path  string
		value any
	}{
		{"diagram/items/0/bounds", map[string]any{"x": 0, "y": 0, "width": 1, "height": 1}},
		{"diagram/items/0/visibility/display", "inline"},
		{"diagram/items/2/textFragments/0/bounds", map[string]any{"x": 0, "y": 0, "width": 1, "height": 1}},
		{"diagram/coordinateSpace", "svg-viewport-css-pixels"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			invalid := decodeSVGFixture(t, raw)
			svgFixtureSet(invalid, tc.path, tc.value)
			body, err := json.Marshal(invalid)
			require.NoError(t, err)
			require.Error(t, validateSVGSnapshot(body, request, identity, run))
		})
	}
}

func TestValidateSVGSnapshotRejectsNonUTF8AndUnboundedNumericTokens(t *testing.T) {
	raw := svgValidationFixture(t, "svg-snapshot-v1.json")
	request, identity, run := svgValidationContext()
	t.Run("UTF8", func(t *testing.T) {
		invalidUTF8 := bytes.Replace(raw, []byte("Example panel"), []byte{0xff}, 1)
		require.Error(t, validateSVGSnapshot(invalidUTF8, request, identity, run))
	})
	for _, number := range []string{"0e4097", "0e-4097", "0e9999999999999999999999999", "0." + strings.Repeat("0", 128)} {
		t.Run("numeric token", func(t *testing.T) {
			invalid := bytes.Replace(raw, []byte("12.3456789012"), []byte(number), 1)
			require.Error(t, validateSVGSnapshot(invalid, request, identity, run))
		})
	}
}

func TestValidateSVGSnapshotChecksThresholdInputs(t *testing.T) {
	value := decodeSVGFixture(t, svgValidationFixture(t, "svg-snapshot-v1.json"))
	input := svgFixtureAt(value, "expressions/0/inputs/0")
	svgFixtureSet(value, "metrics/0/scalar/thresholdTrace/0/inputs", []any{input})
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	request, identity, run := svgValidationContext()
	require.NoError(t, validateSVGSnapshot(raw, request, identity, run))
	for _, tc := range []struct {
		field string
		value any
	}{
		{"value", nil}, {"diagnosticIds", []string{"missing"}},
	} {
		t.Run(tc.field, func(t *testing.T) {
			value := decodeSVGFixture(t, raw)
			svgFixtureSet(value, "metrics/0/scalar/thresholdTrace/0/inputs/0/"+tc.field, tc.value)
			invalid, err := json.Marshal(value)
			require.NoError(t, err)
			require.Error(t, validateSVGSnapshot(invalid, request, identity, run))
		})
	}
}

func TestValidateSVGSnapshotConcurrentCallsPreservePayload(t *testing.T) {
	raw := svgValidationFixture(t, "svg-snapshot-v1.json")
	before := bytes.Clone(raw)
	request, identity, run := svgValidationContext()
	errors := make(chan error, 16)
	for range 16 {
		go func() { errors <- validateSVGSnapshot(raw, request, identity, run) }()
	}
	for range 16 {
		require.NoError(t, <-errors)
	}
	require.Equal(t, before, []byte(raw))
}

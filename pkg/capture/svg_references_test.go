package capture

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func svgRefFixture(t *testing.T) any {
	t.Helper()
	raw, err := os.ReadFile("testdata/svg-snapshot-v2.json")
	require.NoError(t, err)
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var v any
	require.NoError(t, d.Decode(&v))
	return v
}

func TestSvgHelpers(t *testing.T) {
	t.Parallel()
	require.Nil(t, svgObject(nil))
	require.Equal(t, map[string]any{"a": 1}, svgObject(map[string]any{"a": 1}))
	require.Nil(t, svgObject("x"))
}

func TestSvgArrayHelper(t *testing.T) {
	t.Parallel()
	r := map[string]any{"k": []any{1, 2}}
	require.Equal(t, []any{1, 2}, svgArray(r, "k"))
	require.Nil(t, svgArray(map[string]any{}, "k"))
	require.Nil(t, svgArray(map[string]any{"k": nil}, "k"))
}

func TestSvgIntegerHelper(t *testing.T) {
	t.Parallel()
	require.Equal(t, int64(123), svgInteger(json.Number("123")))
	require.Equal(t, int64(12), svgInteger(json.Number("12.7")))
	require.Equal(t, int64(0), svgInteger("x"))
}

func TestSvgContainsHelper(t *testing.T) {
	t.Parallel()
	require.True(t, svgContains([]any{1, 2, 3}, 2))
	require.False(t, svgContains([]any{1, 2, 3}, 9))
	require.False(t, svgContains([]any{}, 1))
	require.False(t, svgContains(nil, 1))
}

func TestSvgSameIndexHelper(t *testing.T) {
	t.Parallel()
	var a, b json.Number = "5", "5"
	require.True(t, svgSameIndex(a, b))
	require.False(t, svgSameIndex(nil, json.Number("1")))
	require.True(t, svgSameIndex(nil, nil))
}

func TestValidateSVGRefs_Valid(t *testing.T) {
	t.Parallel()
	v := svgRefFixture(t)
	require.NoError(t, validateSVGReferences(v.(map[string]any)))
}

func TestValidateSVGRefs_DuplicateIDs(t *testing.T) {
	t.Parallel()
	v := svgRefFixture(t)
	objs := v.(map[string]any)["objects"].([]any)
	dup := map[string]any{"id": "object-a", "kind": "object", "indicatorIds": []any{}, "navigation": []any{}}
	objs = append(objs, dup)
	v.(map[string]any)["objects"] = objs
	err := validateSVGReferences(v.(map[string]any))
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate")
}

func TestValidateSVGRefs_BrokenRef(t *testing.T) {
	t.Parallel()
	v := svgRefFixture(t)
	ind := v.(map[string]any)["indicators"].([]any)[0].(map[string]any)
	ind["metricIds"] = []any{"missing-metric"}
	err := validateSVGReferences(v.(map[string]any))
	require.Error(t, err)
	require.Error(t, err)
}

func TestValidateSVGRefs_ParentMissing(t *testing.T) {
	t.Parallel()
	v := svgRefFixture(t)
	objs := v.(map[string]any)["objects"].([]any)
	objs[0].(map[string]any)["parentId"] = "non-existent"
	err := validateSVGReferences(v.(map[string]any))
	require.Error(t, err)
}

func TestValidateSVGRefs_CycleDiag(t *testing.T) {
	t.Parallel()
	v := svgRefFixture(t)
	diags := v.(map[string]any)["diagnostics"].([]any)
	if len(diags) > 0 {
		first := diags[0].(map[string]any)
		first["causeIds"] = []any{first["id"]}
		err := validateSVGReferences(v.(map[string]any))
		require.Error(t, err)
		require.Contains(t, err.Error(), "cycle")
	}
}

func TestValidateSVGRefs_NavLinkMissing(t *testing.T) {
	t.Parallel()
	v := svgRefFixture(t)
	ind := v.(map[string]any)["indicators"].([]any)[0].(map[string]any)
	navs := ind["navigation"].([]any)
	if len(navs) > 0 {
		nav := navs[0].(map[string]any)
		nav["linkId"] = "non-existent"
		ind["navigation"] = navs
	}
	err := validateSVGReferences(v.(map[string]any))
	require.Error(t, err)
}

func TestValidateSVGRefs_RuleMissing(t *testing.T) {
	t.Parallel()
	v := svgRefFixture(t)
	ind := v.(map[string]any)["indicators"].([]any)[0].(map[string]any)
	rrs := ind["ruleResults"].([]any)
	if len(rrs) > 0 {
		rr := rrs[0].(map[string]any)
		rr["ruleId"] = "non-existent-rule"
		ind["ruleResults"] = rrs
	}
	err := validateSVGReferences(v.(map[string]any))
	require.Error(t, err)
}

func TestValidateSVGRefs_ValidWithExpressions(t *testing.T) {
	t.Parallel()
	v := svgRefFixture(t)
	// Add expressions with inputs to cover input() function
	v.(map[string]any)["expressions"] = []any{
		map[string]any{
			"id":            "expr-1",
			"availability":  "available",
			"value":         123,
			"diagnosticIds": []any{},
			"inputs": []any{
				map[string]any{
					"availability":  "available",
					"value":         456,
					"diagnosticIds": []any{},
				},
			},
		},
	}
	require.NoError(t, validateSVGReferences(v.(map[string]any)))
}

func TestValidateSVGRefs_NavigationWithRuleId(t *testing.T) {
	t.Parallel()
	v := svgRefFixture(t)
	// Add navigation with ruleId to cover ruleId check
	ind := v.(map[string]any)["indicators"].([]any)[0].(map[string]any)
	navs := []any{
		map[string]any{"linkId": "link-a", "use": "applied", "ruleId": "rule-a"},
	}
	ind["navigation"] = navs
	require.NoError(t, validateSVGReferences(v.(map[string]any)))
}

func TestValidateSVGRefs_AppliedThresholdWithInputs(t *testing.T) {
	t.Parallel()
	v := svgRefFixture(t)
	// Set appliedThreshold.inputs to cover decision inputs
	scalar := v.(map[string]any)["metrics"].([]any)[0].(map[string]any)["scalar"].(map[string]any)
	appliedThreshold := scalar["appliedThreshold"].(map[string]any)
	appliedThreshold["inputs"] = []any{
		map[string]any{
			"availability":  "available",
			"value":         10,
			"diagnosticIds": []any{},
		},
	}
	scalar["appliedThreshold"] = appliedThreshold
	require.NoError(t, validateSVGReferences(v.(map[string]any)))
}

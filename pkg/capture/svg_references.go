package capture

import (
	"encoding/json"
	"fmt"
)

// Called only after schema validation. Missing referenced records still return nil, never panic.
type svgRecord = map[string]any
type svgIndex = map[string]svgRecord

func svgObject(value any) svgRecord               { v, _ := value.(map[string]any); return v }
func svgArray(record svgRecord, key string) []any { v, _ := record[key].([]any); return v }
func svgInteger(value any) int64                  { n, _ := value.(json.Number); f, _ := n.Float64(); return int64(f) }
func svgContains(values []any, target any) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}
func svgSameIndex(a, b any) bool {
	if a == nil || b == nil {
		return a == b
	}
	return svgInteger(a) == svgInteger(b)
}

type svgReferenceValidator struct {
	err                                                     error
	objects, indicators, metrics, rules, links, diagnostics svgIndex
}

func (v *svgReferenceValidator) require(ok bool, category string) {
	if !ok && v.err == nil {
		v.err = fmt.Errorf("invalid SVG snapshot %s", category)
	}
}
func (v *svgReferenceValidator) index(rows []any) svgIndex {
	out := svgIndex{}
	for _, r := range rows {
		row := svgObject(r)
		id := row["id"].(string)
		v.require(out[id] == nil, "duplicate ID")
		out[id] = row
	}
	return out
}
func (v *svgReferenceValidator) refs(ids []any, index svgIndex) {
	for _, id := range ids {
		v.require(index[id.(string)] != nil, "reference")
	}
}
func (v *svgReferenceValidator) diags(row svgRecord) {
	v.refs(svgArray(row, "diagnosticIds"), v.diagnostics)
}
func (v *svgReferenceValidator) offset(n any, length int) bool {
	ok := n == nil || svgInteger(n) < int64(length)
	v.require(ok, "index")
	return ok
}
func (v *svgReferenceValidator) orderedRange(a, b any) {
	v.require(a == nil || b == nil || svgInteger(a) <= svgInteger(b), "time range")
}
func (v *svgReferenceValidator) input(row svgRecord) {
	v.diags(row)
	v.require((row["availability"] == "available") == (row["value"] != nil), "availability")
}
func (v *svgReferenceValidator) paint(value any) {
	if value != nil {
		p := svgObject(value)
		v.require((p["kind"] == "solid") == (p["rgba"] != nil), "paint")
	}
}
func (v *svgReferenceValidator) navigation(row svgRecord) {
	for _, raw := range svgArray(row, "navigation") {
		n := svgObject(raw)
		v.refs([]any{n["linkId"]}, v.links)
		if n["ruleId"] != nil {
			v.refs([]any{n["ruleId"]}, v.rules)
		}
	}
}
func (v *svgReferenceValidator) decision(row svgRecord) {
	v.paint(row["color"])
	if t := svgObject(row["appliedThreshold"]); t != nil {
		v.paint(t["color"])
		v.diags(t)
		for _, input := range svgArray(t, "inputs") {
			v.input(svgObject(input))
		}
	}
}
func (v *svgReferenceValidator) noCycles(index svgIndex, next func(svgRecord) []any) {
	colors := map[string]byte{}
	var visit func(string)
	visit = func(id string) {
		if colors[id] == 1 {
			v.require(false, "cycle")
			return
		}
		if colors[id] == 2 {
			return
		}
		colors[id] = 1
		if row := index[id]; row != nil {
			for _, child := range next(row) {
				visit(child.(string))
			}
		}
		colors[id] = 2
	}
	for id := range index {
		visit(id)
	}
}
func validateSVGReferences(snapshot svgRecord) error {
	v := &svgReferenceValidator{}
	v.objects = v.index(svgArray(snapshot, "objects"))
	v.indicators = v.index(svgArray(snapshot, "indicators"))
	v.metrics = v.index(svgArray(snapshot, "metrics"))
	v.rules = v.index(svgArray(snapshot, "rules"))
	v.links = v.index(svgArray(snapshot, "links"))
	v.diagnostics = v.index(svgArray(snapshot, "diagnostics"))
	v.index(svgArray(snapshot, "expressions"))
	if v.err != nil {
		return v.err
	}
	observed := svgObject(snapshot["observed"])
	v.orderedRange(observed["effectiveFromMs"], observed["effectiveToMs"])
	for _, o := range v.objects {
		v.refs(svgArray(o, "indicatorIds"), v.indicators)
		v.navigation(o)
		if o["parentId"] != nil {
			v.refs([]any{o["parentId"]}, v.objects)
		}
		v.require((o["parentId"] == nil) == (o["parentRelation"] == nil), "parent relation")
		for _, id := range svgArray(o, "indicatorIds") {
			v.require(svgContains(svgArray(v.indicators[id.(string)], "objectIds"), o["id"]), "object binding")
		}
	}
	v.noCycles(v.objects, func(o svgRecord) []any {
		if o["parentId"] == nil {
			return nil
		}
		return []any{o["parentId"]}
	})
	for _, r := range v.rules {
		v.refs(svgArray(r, "indicatorIds"), v.indicators)
		v.diags(r)
		v.navigation(r)
	}
	for _, m := range v.metrics {
		v.metric(m)
	}
	for _, i := range v.indicators {
		v.indicator(i)
	}
	for _, d := range v.diagnostics {
		v.refs(svgArray(d, "indicatorIds"), v.indicators)
		v.refs(svgArray(d, "ruleIds"), v.rules)
		v.refs(svgArray(d, "metricIds"), v.metrics)
		v.refs(svgArray(d, "causeIds"), v.diagnostics)
	}
	v.noCycles(v.diagnostics, func(d svgRecord) []any { return svgArray(d, "causeIds") })
	for _, e := range svgArray(snapshot, "expressions") {
		r := svgObject(e)
		v.input(r)
		for _, input := range svgArray(r, "inputs") {
			v.input(svgObject(input))
		}
	}
	return v.err
}
func (v *svgReferenceValidator) metric(m svgRecord) {
	v.refs([]any{m["ruleId"]}, v.rules)
	v.refs(svgArray(m, "indicatorIds"), v.indicators)
	v.diags(m)
	switch m["kind"] {
	case "scalar":
		v.require(m["table"] == nil && (m["availability"] == "available") == (m["scalar"] != nil), "scalar state")
	case "table":
		v.require(m["scalar"] == nil && m["table"] != nil, "table state")
	default:
		v.require(m["availability"] == "unavailable" && m["scalar"] == nil && m["table"] == nil, "unresolved state")
	}
	if s := svgObject(m["scalar"]); s != nil {
		v.decision(s)
	}
	for _, raw := range svgArray(m, "sources") {
		s := svgObject(raw)
		v.diags(s)
		v.orderedRange(s["fromMs"], s["toMs"])
	}
	for _, id := range svgArray(m, "indicatorIds") {
		v.require(svgContains(svgArray(v.indicators[id.(string)], "metricIds"), m["id"]), "metric binding")
	}
	if t := svgObject(m["table"]); t != nil {
		rows, columns := svgArray(t, "rows"), svgArray(t, "columns")
		v.offset(t["thresholdColumnIndex"], len(columns))
		winnerOK := v.offset(t["winningRowIndex"], len(rows))
		sources := map[int64]bool{}
		for _, raw := range rows {
			r := svgObject(raw)
			i := svgInteger(r["sourceIndex"])
			v.require(!sources[i], "duplicate source index")
			sources[i] = true
			v.require(len(svgArray(r, "values")) == len(columns) && len(svgArray(r, "displayValues")) == len(columns), "table width")
			if d := svgObject(r["decision"]); d != nil {
				v.decision(d)
			}
			for _, raw := range svgArray(r, "cellIssues") {
				issue := svgObject(raw)
				values := svgArray(r, "values")
				if v.offset(issue["columnIndex"], len(values)) {
					v.require(values[svgInteger(issue["columnIndex"])] == nil, "cell issue")
				}
				v.diags(issue)
			}
		}
		if winnerOK && t["winningRowIndex"] != nil {
			v.require(svgObject(rows[svgInteger(t["winningRowIndex"])])["decision"] != nil && m["availability"] == "available", "table winner")
		}
	}
}
func (v *svgReferenceValidator) winner(id, row any, assigned []any) {
	if id == nil {
		v.require(row == nil, "winner row")
		return
	}
	m := v.metrics[id.(string)]
	if m == nil {
		v.require(false, "winner metric")
		return
	}
	v.require(svgContains(assigned, id) && m["availability"] == "available", "winner assignment")
	if m["kind"] == "scalar" {
		v.require(row == nil && m["scalar"] != nil, "scalar winner")
	} else {
		v.require(m["kind"] == "table" && row != nil && svgSameIndex(svgObject(m["table"])["winningRowIndex"], row), "table winner")
	}
}
func (v *svgReferenceValidator) indicator(i svgRecord) {
	v.refs(svgArray(i, "objectIds"), v.objects)
	binding := svgObject(i["binding"])
	v.refs(svgArray(binding, "candidateObjectIds"), v.objects)
	v.refs(svgArray(i, "metricIds"), v.metrics)
	v.diags(i)
	v.navigation(i)
	for _, id := range svgArray(i, "objectIds") {
		v.require(svgContains(svgArray(v.objects[id.(string)], "indicatorIds"), i["id"]), "indicator binding")
	}
	if binding["status"] == "ambiguous" || binding["status"] == "unresolved" {
		v.require(len(svgArray(i, "objectIds")) == 0, "binding status")
	}
	for _, raw := range svgArray(i, "appearance") {
		p := svgObject(raw)
		v.paint(p["fill"])
		v.paint(p["stroke"])
		v.paint(p["textColor"])
	}
	s := svgObject(i["state"])
	v.paint(s["color"])
	if s["selectedRuleId"] != nil {
		v.refs([]any{s["selectedRuleId"]}, v.rules)
	}
	v.winner(s["winnerMetricId"], s["winnerRowIndex"], svgArray(i, "metricIds"))
	v.require(s["noData"] != true || s["winnerMetricId"] == nil, "no data winner")
	for _, id := range svgArray(i, "metricIds") {
		v.require(svgContains(svgArray(v.metrics[id.(string)], "indicatorIds"), i["id"]), "indicator metric binding")
	}
	selected := s["selectedRuleId"] == nil && s["winnerMetricId"] == nil && s["winnerRowIndex"] == nil
	for _, raw := range svgArray(i, "ruleResults") {
		r := svgObject(raw)
		v.refs([]any{r["ruleId"]}, v.rules)
		v.refs(svgArray(r, "metricIds"), v.metrics)
		v.winner(r["winnerMetricId"], r["winnerRowIndex"], svgArray(r, "metricIds"))
		v.require(svgContains(svgArray(v.rules[r["ruleId"].(string)], "indicatorIds"), i["id"]), "rule binding")
		for _, id := range svgArray(r, "metricIds") {
			v.require(v.metrics[id.(string)]["ruleId"] == r["ruleId"] && svgContains(svgArray(i, "metricIds"), id), "rule metric")
		}
		if r["ruleId"] == s["selectedRuleId"] && r["winnerMetricId"] == s["winnerMetricId"] && svgSameIndex(r["winnerRowIndex"], s["winnerRowIndex"]) {
			selected = true
		}
	}
	v.require(selected, "selected winner")
	t := svgObject(i["tooltip"])
	v.diags(t)
	v.refs(svgArray(t, "metricIds"), v.metrics)
	for _, id := range svgArray(t, "metricIds") {
		v.require(svgContains(svgArray(i, "metricIds"), id), "tooltip assignment")
	}
	for _, raw := range svgArray(t, "tables") {
		r := svgObject(raw)
		id := r["metricId"].(string)
		m := v.metrics[id]
		v.require(m != nil && m["kind"] == "table" && svgContains(svgArray(i, "metricIds"), id), "tooltip table")
		for _, n := range svgArray(r, "rowIndices") {
			v.offset(n, len(svgArray(svgObject(m["table"]), "rows")))
		}
	}
}

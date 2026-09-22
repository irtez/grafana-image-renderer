package capture

import (
	"encoding/json"
	"fmt"
)

// Эти представления используются только после полной проверки JSON Schema.
type svgRecord = map[string]any
type svgIndex = map[string]svgRecord

func svgObject(value any) svgRecord               { return value.(map[string]any) }
func svgArray(record svgRecord, key string) []any { return record[key].([]any) }
func svgInteger(value any) int64 {
	// Все структурные индексы уже ограничены схемой безопасными целыми JS.
	number, _ := value.(json.Number).Float64()
	return int64(number)
}

func svgContains(values []any, target any) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func svgSameIndex(left, right any) bool {
	if left == nil || right == nil {
		return left == right
	}
	return svgInteger(left) == svgInteger(right)
}

type svgReferenceValidator struct {
	err                                          error
	rules, elements, metrics, diagnostics, items svgIndex
}

func (v *svgReferenceValidator) require(condition bool, category string) {
	if !condition && v.err == nil {
		v.err = fmt.Errorf("invalid SVG snapshot %s", category)
	}
}

func (v *svgReferenceValidator) index(values []any) svgIndex {
	index := make(svgIndex, len(values))
	for _, value := range values {
		item := svgObject(value)
		id := item["id"].(string)
		v.require(index[id] == nil, "duplicate ID")
		index[id] = item
	}
	return index
}

func (v *svgReferenceValidator) refs(ids []any, target svgIndex) {
	for _, id := range ids {
		v.require(target[id.(string)] != nil, "reference")
	}
}

func (v *svgReferenceValidator) diags(record svgRecord) {
	v.refs(svgArray(record, "diagnosticIds"), v.diagnostics)
}

func (v *svgReferenceValidator) orderedRange(from, to any) {
	v.require(from == nil || to == nil || svgInteger(from) <= svgInteger(to), "time range")
}

func (v *svgReferenceValidator) offset(position any, length int) bool {
	ok := position == nil || svgInteger(position) < int64(length)
	v.require(ok, "index")
	return ok
}

func (v *svgReferenceValidator) availability(record svgRecord) {
	v.require((record["availability"] == "available") == (record["value"] != nil), "availability")
}

func (v *svgReferenceValidator) input(record svgRecord) {
	v.diags(record)
	v.availability(record)
}

func validateSVGReferences(snapshot svgRecord) error {
	v := &svgReferenceValidator{}
	configuration := svgObject(snapshot["configuration"])
	diagram := svgObject(snapshot["diagram"])
	v.rules = v.index(svgArray(configuration, "rules"))
	v.elements = v.index(svgArray(snapshot, "elements"))
	v.metrics = v.index(svgArray(snapshot, "metrics"))
	v.diagnostics = v.index(svgArray(snapshot, "diagnostics"))
	v.items = v.index(svgArray(diagram, "items"))
	v.index(svgArray(snapshot, "expressions"))
	v.index(svgArray(diagram, "connections"))
	if v.err != nil {
		return v.err
	}

	observed := svgObject(snapshot["observed"])
	v.orderedRange(observed["effectiveFromMs"], observed["effectiveToMs"])
	v.diags(configuration)
	v.diags(diagram)
	for _, rule := range v.rules {
		v.diags(rule)
		v.refs(svgArray(rule, "elementIds"), v.elements)
		for _, elementID := range svgArray(rule, "elementIds") {
			element := v.elements[elementID.(string)]
			v.require(element != nil && svgRuleResult(element, rule["id"]) != nil, "rule binding")
		}
	}
	for _, metric := range v.metrics {
		v.metric(metric)
	}
	for _, element := range v.elements {
		v.element(element)
	}
	for _, value := range svgArray(snapshot, "expressions") {
		expression := svgObject(value)
		v.input(expression)
		for _, input := range svgArray(expression, "inputs") {
			v.input(svgObject(input))
		}
	}
	for _, diagnostic := range v.diagnostics {
		v.refs(svgArray(diagnostic, "ruleIds"), v.rules)
		v.refs(svgArray(diagnostic, "elementIds"), v.elements)
		v.refs(svgArray(diagnostic, "metricIds"), v.metrics)
	}
	v.diagram(diagram)
	return v.err
}

func svgRuleResult(element svgRecord, ruleID any) svgRecord {
	for _, value := range svgArray(element, "ruleResults") {
		result := svgObject(value)
		if result["ruleId"] == ruleID {
			return result
		}
	}
	return nil
}

func (v *svgReferenceValidator) decision(result, metric svgRecord) {
	trace := svgArray(result, "thresholdTrace")
	indices := make(map[int64]bool, len(trace))
	thresholds, isArray := svgObject(metric["settings"])["thresholds"].([]any)
	var selected any
	for _, value := range trace {
		check := svgObject(value)
		index := svgInteger(check["index"])
		v.require(!indices[index], "duplicate threshold index")
		indices[index] = true
		v.diags(check)
		for _, input := range svgArray(check, "inputs") {
			v.input(svgObject(input))
		}
		v.require(isArray && index < int64(len(thresholds)), "threshold index")
		matched := (check["condition"] == "true" || check["condition"] == "not_present") && check["comparison"] == "true"
		v.require(check["matched"] == matched, "threshold trace")
		if check["matched"] == true {
			selected = check["index"]
		}
	}
	// Сверяем записанный след, не вычисляя пороги, reducers, уровень или цвет.
	v.require(svgSameIndex(result["selectedThresholdIndex"], selected), "selected threshold")
}

func (v *svgReferenceValidator) metric(metric svgRecord) {
	v.diags(metric)
	rule := v.rules[metric["ruleId"].(string)]
	v.require(rule != nil, "rule reference")
	v.refs(svgArray(metric, "elementIds"), v.elements)
	if rule != nil {
		if configured, ok := svgObject(rule["attributes"])["metrics"].([]any); ok {
			if v.offset(metric["metricsIndex"], len(configured)) {
				entry := configured[svgInteger(metric["metricsIndex"])]
				if object, ok := entry.(map[string]any); ok {
					if queries, ok := object["queries"].([]any); ok {
						v.offset(metric["queryIndex"], len(queries))
					}
				}
			}
		}
	}
	for _, value := range svgArray(metric, "sources") {
		source := svgObject(value)
		v.diags(source)
		v.orderedRange(source["fromMs"], source["toMs"])
	}
	available := metric["availability"] == "available"
	switch metric["kind"] {
	case "scalar":
		v.require(metric["table"] == nil && available == (metric["scalar"] != nil), "scalar state")
	case "table":
		v.require(metric["scalar"] == nil && (!available || metric["table"] != nil), "table state")
	default:
		v.require(!available && metric["scalar"] == nil && metric["table"] == nil, "unresolved state")
	}
	if metric["scalar"] != nil {
		v.decision(svgObject(metric["scalar"]), metric)
	}
	if metric["table"] != nil {
		v.table(svgObject(metric["table"]), metric)
	}
	for _, elementID := range svgArray(metric, "elementIds") {
		element := v.elements[elementID.(string)]
		var result svgRecord
		if element != nil {
			result = svgRuleResult(element, metric["ruleId"])
		}
		v.require(result != nil && svgContains(svgArray(result, "metricIds"), metric["id"]), "metric binding")
	}
}

func (v *svgReferenceValidator) table(table, metric svgRecord) {
	columns, rows := svgArray(table, "columns"), svgArray(table, "rows")
	v.offset(table["thresholdColumnIndex"], len(columns))
	winnerInRange := v.offset(table["winningRowIndex"], len(rows))
	v.require(metric["availability"] == "available" || table["winningRowIndex"] == nil, "table winner")
	sourceIndices := make(map[int64]bool, len(rows))
	for _, value := range rows {
		row := svgObject(value)
		index := svgInteger(row["sourceIndex"])
		v.require(!sourceIndices[index], "duplicate source index")
		sourceIndices[index] = true
		values := svgArray(row, "values")
		v.require(len(values) == len(columns) && len(svgArray(row, "displayValues")) == len(columns), "table width")
		for _, value := range svgArray(row, "cellIssues") {
			issue := svgObject(value)
			column := svgInteger(issue["columnIndex"])
			v.offset(issue["columnIndex"], len(columns))
			v.require(len(svgArray(issue, "diagnosticIds")) > 0, "cell issue diagnostics")
			v.require(column < int64(len(values)) && values[column] == nil, "cell issue value")
			v.diags(issue)
		}
		if row["decision"] != nil {
			v.require(table["thresholdColumnIndex"] != nil, "table decision column")
			v.decision(svgObject(row["decision"]), metric)
		}
	}
	if winnerInRange && table["winningRowIndex"] != nil {
		row := svgObject(rows[svgInteger(table["winningRowIndex"])])
		v.require(row["decision"] != nil, "table winner decision")
	}
}

func (v *svgReferenceValidator) winner(element svgRecord, metricID, rowIndex any, allowed []any) {
	if metricID == nil {
		v.require(rowIndex == nil, "winner row")
		return
	}
	metric := v.metrics[metricID.(string)]
	if metric == nil {
		v.require(false, "winner reference")
		return
	}
	v.require(metric["availability"] == "available" && svgContains(allowed, metricID) &&
		svgContains(svgArray(metric, "elementIds"), element["id"]), "winner binding")
	if metric["kind"] == "scalar" {
		v.require(metric["scalar"] != nil && rowIndex == nil, "scalar winner")
	} else {
		v.require(metric["kind"] == "table" && rowIndex != nil && metric["table"] != nil &&
			svgSameIndex(rowIndex, svgObject(metric["table"])["winningRowIndex"]), "table winner")
	}
}

func (v *svgReferenceValidator) element(element svgRecord) {
	v.diags(element)
	if element["diagramItemId"] != nil {
		item := v.items[element["diagramItemId"].(string)]
		v.require(item != nil && item["svgId"] == element["id"], "diagram binding")
	}
	ruleIDs := make(map[string]bool)
	for _, value := range svgArray(element, "ruleResults") {
		result := svgObject(value)
		id := result["ruleId"].(string)
		v.require(!ruleIDs[id], "duplicate rule result")
		ruleIDs[id] = true
		rule := v.rules[id]
		v.require(rule != nil && svgContains(svgArray(rule, "elementIds"), element["id"]), "element rule binding")
		for _, metricID := range svgArray(result, "metricIds") {
			metric := v.metrics[metricID.(string)]
			v.require(metric != nil && metric["ruleId"] == result["ruleId"] &&
				svgContains(svgArray(metric, "elementIds"), element["id"]), "element metric binding")
		}
		v.winner(element, result["winnerMetricId"], result["winnerRowIndex"], svgArray(result, "metricIds"))
	}
	selected := svgRuleResult(element, element["selectedRuleId"])
	v.require(element["selectedRuleId"] == nil || selected != nil, "selected rule")
	var allowed []any
	var metricID, rowIndex any
	if selected != nil {
		allowed = svgArray(selected, "metricIds")
		metricID, rowIndex = selected["winnerMetricId"], selected["winnerRowIndex"]
	}
	v.winner(element, element["winnerMetricId"], element["winnerRowIndex"], allowed)
	v.require(metricID == element["winnerMetricId"] && svgSameIndex(rowIndex, element["winnerRowIndex"]), "selected winner")
	v.require(element["noData"] == nil || element["winnerMetricId"] == nil, "no-data winner")
}

func (v *svgReferenceValidator) diagram(diagram svgRecord) {
	rendered := diagram["status"] == "rendered"
	v.require(rendered == (diagram["viewport"] != nil && diagram["coordinateSpace"] != nil), "diagram state")
	if !rendered {
		v.require(diagram["viewport"] == nil && diagram["coordinateSpace"] == nil, "diagram state")
	}
	for _, item := range v.items {
		v.diags(item)
		if item["parentId"] != nil {
			v.require(v.items[item["parentId"].(string)] != nil, "parent reference")
		}
		if !rendered {
			v.require(item["bounds"] == nil && len(svgArray(item, "paints")) == 0, "unrendered item")
			for _, text := range svgArray(item, "textFragments") {
				v.require(svgObject(text)["bounds"] == nil, "unrendered text")
			}
			for _, value := range svgObject(item["visibility"]) {
				v.require(value == nil, "unrendered visibility")
			}
		}
		for _, value := range svgArray(svgObject(item["links"]), "declarations") {
			link := svgObject(value)
			v.require((link["origin"] == "rule") == (link["ruleId"] != nil), "link binding")
			if link["ruleId"] != nil {
				v.require(v.rules[link["ruleId"].(string)] != nil, "link rule reference")
			}
		}
	}
	v.parentCycles()
	for _, value := range svgArray(diagram, "connections") {
		connection := svgObject(value)
		if connection["itemId"] != nil {
			v.require(v.items[connection["itemId"].(string)] != nil, "connection item reference")
		}
		for _, side := range []string{"source", "target"} {
			endpoint := svgObject(connection[side])
			if endpoint["itemId"] != nil {
				item := v.items[endpoint["itemId"].(string)]
				v.require(item != nil && (endpoint["cellId"] == nil || endpoint["cellId"] == item["cellId"]) &&
					(endpoint["svgId"] == nil || endpoint["svgId"] == item["svgId"]), "connection endpoint binding")
			}
		}
	}
}

func (v *svgReferenceValidator) parentCycles() {
	// Три состояния дают линейный обход даже для длинной цепочки родителей.
	state := make(map[string]uint8, len(v.items))
	for id := range v.items {
		current := id
		var path []string
		for v.items[current] != nil && state[current] == 0 {
			state[current] = 1
			path = append(path, current)
			parent := v.items[current]["parentId"]
			if parent == nil {
				current = ""
				break
			}
			current = parent.(string)
		}
		v.require(state[current] != 1, "parent cycle")
		for _, ancestor := range path {
			state[ancestor] = 2
		}
	}
}

package capture

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Схема компилируется только при первом SVG capture; обычный render её не загружает.
//
//go:embed svgmodifier-snapshot-v1.schema.json
var svgSnapshotSchema []byte

var compiledSVGSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(svgSnapshotSchema))
	if err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	const resource = "urn:svgmodifier:snapshot:v1"
	if err := compiler.AddResource(resource, document); err != nil {
		return nil, err
	}
	return compiler.Compile(resource)
})

const (
	svgMaxJSONDepth  = 64
	svgMaxJSONValues = 100000
	// JSON.stringify конечного JS number укладывается с большим запасом.
	// Эти пределы также ограничивают big.Rat внутри JSON Schema validator.
	svgMaxNumberLength = 128
	svgMaxExponent     = 4096
)

func validateSVGSnapshot(raw json.RawMessage, request Request, identity SVGIdentity, run SVGRun) error {
	if !utf8.Valid(raw) {
		return fmt.Errorf("invalid SVG snapshot JSON encoding")
	}
	// UseNumber сохраняет исходные числовые токены; наружу передаётся сам raw.
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("invalid SVG snapshot JSON")
	}
	remaining := svgMaxJSONValues
	if !svgBoundedJSON(value, 0, &remaining) {
		return fmt.Errorf("SVG snapshot JSON exceeds validation limits or contains a non-finite number")
	}
	schema, err := compiledSVGSchema()
	if err != nil {
		return fmt.Errorf("SVG snapshot schema is unavailable")
	}
	if err := schema.Validate(value); err != nil {
		// Ошибка библиотеки может содержать исходные строки: не возвращаем её клиенту.
		return fmt.Errorf("SVG snapshot does not match schema v1")
	}

	snapshot := value.(map[string]any)
	producer := svgObject(snapshot["producer"])
	panel := svgObject(snapshot["panel"])
	observed := svgObject(snapshot["observed"])
	if request.Kind != "svgmodifier" || identity.ProducerID != producer["id"] ||
		identity.ProducerVersion != producer["version"] || identity.InstanceID == "" ||
		identity.PanelID != request.PanelID || svgInteger(panel["id"]) != int64(request.PanelID) {
		return fmt.Errorf("SVG snapshot identity does not match capture")
	}
	if svgInteger(observed["generation"]) != run.Generation ||
		svgInteger(observed["effectiveFromMs"]) != run.EffectiveFromMs ||
		svgInteger(observed["effectiveToMs"]) != run.EffectiveToMs {
		return fmt.Errorf("SVG snapshot does not match active run")
	}
	// Фактическое окно принадлежит producer: panel overrides могут менять render range.
	return validateSVGReferences(snapshot)
}

func svgBoundedJSON(value any, depth int, remaining *int) bool {
	*remaining--
	if depth > svgMaxJSONDepth || *remaining < 0 {
		return false
	}
	switch value := value.(type) {
	case json.Number:
		if len(value) > svgMaxNumberLength {
			return false
		}
		if index := strings.IndexAny(string(value), "eE"); index >= 0 {
			exponent, err := strconv.ParseInt(string(value[index+1:]), 10, 64)
			if err != nil || exponent < -svgMaxExponent || exponent > svgMaxExponent {
				return false
			}
		}
		number, err := value.Float64()
		return err == nil && !math.IsInf(number, 0) && !math.IsNaN(number)
	case []any:
		for _, child := range value {
			if !svgBoundedJSON(child, depth+1, remaining) {
				return false
			}
		}
	case map[string]any:
		for _, child := range value {
			if !svgBoundedJSON(child, depth+1, remaining) {
				return false
			}
		}
	}
	return true
}

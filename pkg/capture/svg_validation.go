package capture

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Схема компилируется только при первом SVG capture; обычный render её не загружает.
//
//go:embed svgmodifier-snapshot-v2.schema.json
var svgSnapshotSchema []byte

var compiledSVGSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(svgSnapshotSchema))
	if err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	const resource = "urn:svgmodifier:snapshot:v2"
	if err := compiler.AddResource(resource, document); err != nil {
		return nil, err
	}
	return compiler.Compile(resource)
})

const (
	svgMaxJSONDepth = 64
	// JSON.stringify конечного JS number укладывается с большим запасом.
	// Эти пределы также ограничивают big.Rat внутри JSON Schema validator.
	svgMaxNumberLength = 128
	svgMaxExponent     = 4096
)

var errSVGValidationLimit = errors.New("SVG payload exceeds validation limits")

func validateSVGSnapshot(raw json.RawMessage, request Request, identity SVGIdentity, run SVGRun) error {
	if !utf8.Valid(raw) {
		return fmt.Errorf("invalid SVG snapshot JSON encoding")
	}
	if err := preflightSVGJSON(raw); err != nil {
		return err
	}
	// UseNumber сохраняет исходные числовые токены; наружу передаётся сам raw.
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("invalid SVG snapshot JSON")
	}
	remaining := len(raw)
	if !svgBoundedJSON(value, 0, &remaining) {
		return fmt.Errorf("SVG snapshot JSON exceeds validation limits or contains a non-finite number")
	}
	schema, err := compiledSVGSchema()
	if err != nil {
		return fmt.Errorf("SVG snapshot schema is unavailable")
	}
	if err := schema.Validate(value); err != nil {
		// Ошибка библиотеки может содержать исходные строки: не возвращаем её клиенту.
		return fmt.Errorf("SVG snapshot does not match schema v2")
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

// Bound depth/numbers and reject duplicate keys before allocating the schema model.
func preflightSVGJSON(raw []byte) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	remaining := len(raw)
	var walk func(int) error
	walk = func(depth int) error {
		remaining--
		if depth > svgMaxJSONDepth || remaining < 0 {
			return errSVGValidationLimit
		}
		token, err := d.Token()
		if err != nil {
			return fmt.Errorf("invalid SVG JSON")
		}
		switch value := token.(type) {
		case json.Delim:
			if value != '{' && value != '[' {
				return fmt.Errorf("invalid SVG JSON")
			}
			keys := map[string]bool{}
			for d.More() {
				if value == '{' {
					key, err := d.Token()
					if err != nil {
						return fmt.Errorf("invalid SVG JSON")
					}
					s, ok := key.(string)
					if !ok || keys[s] {
						return fmt.Errorf("duplicate SVG JSON key")
					}
					keys[s] = true
				}
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
			if _, err := d.Token(); err != nil {
				return fmt.Errorf("invalid SVG JSON")
			}
		case json.Number:
			if len(value) > svgMaxNumberLength {
				return errSVGValidationLimit
			}
			if index := strings.IndexAny(string(value), "eE"); index >= 0 {
				exp, err := strconv.ParseInt(string(value[index+1:]), 10, 64)
				if err != nil || exp < -svgMaxExponent || exp > svgMaxExponent {
					return errSVGValidationLimit
				}
			}
		}
		return nil
	}
	if err := walk(0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return fmt.Errorf("invalid SVG JSON trailing data")
	}
	return nil
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

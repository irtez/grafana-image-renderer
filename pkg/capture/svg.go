package capture

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/chromedp/cdproto/page"
)

type SVGCollector struct{}

func NewSVGCollector() *SVGCollector              { return &SVGCollector{} }
func (*SVGCollector) Kind() string                { return "svgmodifier" }
func (*SVGCollector) UsesProducerReadiness() bool { return true }

func (*SVGCollector) Initialize(ctx context.Context, request Request, maxBytes int) error {
	if request.Kind != "svgmodifier" || request.PanelID < 0 || maxBytes <= 0 {
		return fmt.Errorf("invalid SVG capture initialization")
	}
	// Без worldName и top-only guard: Grafana sandbox выполняет producer в дочернем realm.
	ids := request.PanelIDs
	if len(ids) == 0 {
		ids = []int{request.PanelID}
	}
	_, err := page.AddScriptToEvaluateOnNewDocument(svgBatchBootstrapScript(ids, maxBytes)).Do(ctx)
	return err
}

type svgScriptError struct {
	Code string `json:"code"`
}

type svgScriptResult struct {
	Unchanged    bool            `json:"unchanged,omitempty"`
	Status       string          `json:"status"`
	Identity     *SVGIdentity    `json:"identity"`
	Run          *SVGRun         `json:"run"`
	SnapshotJSON *string         `json:"snapshotJSON"`
	PayloadBytes int             `json:"payloadBytes"`
	Error        *svgScriptError `json:"error"`
}

func (*SVGCollector) Collect(ctx context.Context, request Request, maxBytes int) (Collection, error) {
	return collectSVGBatch(ctx, request, maxBytes, func(ctx context.Context, focus int, stamps map[int]svgStamp) (svgBatchStep, error) {
		return readSVGBatch(ctx, request, focus, stamps)
	})
}

func svgCollectionFromScript(state svgScriptResult, request Request, maxBytes int) (Collection, bool) {
	invalid := func() (Collection, bool) { return svgError("CAPTURE_PAYLOAD_INVALID"), true }
	if maxBytes <= 0 {
		return invalid()
	}
	switch state.Status {
	case "idle", "pending":
		if state.SnapshotJSON != nil || state.Error != nil || state.PayloadBytes != 0 {
			return invalid()
		}
		return Collection{}, false
	case "terminal-error":
		if state.Error == nil || state.SnapshotJSON != nil || state.PayloadBytes != 0 {
			return invalid()
		}
		return svgError(state.Error.Code), true
	case "terminal-ok":
		if state.Identity == nil || state.Run == nil || state.SnapshotJSON == nil || state.Error != nil {
			return invalid()
		}
		if len(*state.SnapshotJSON) > maxBytes || state.PayloadBytes > maxBytes {
			return svgError("CAPTURE_PAYLOAD_TOO_LARGE"), true
		}
		if state.PayloadBytes <= 0 || state.PayloadBytes != len(*state.SnapshotJSON) {
			return invalid()
		}
		raw := json.RawMessage(*state.SnapshotJSON)
		if err := validateSVGSnapshot(raw, request, *state.Identity, *state.Run); err != nil {
			if errors.Is(err, errSVGValidationLimit) {
				return svgError("CAPTURE_VALIDATION_LIMIT"), true
			}
			return invalid()
		}
		return Collection{Payload: raw}, true
	default:
		return invalid()
	}
}

func svgError(code string) Collection {
	messages := map[string]string{
		"CAPTURE_INTERNAL_ERROR":         "semantic capture failed",
		"CAPTURE_PANEL_NOT_FOUND":        "requested panel was not found",
		"CAPTURE_PANEL_UNSUPPORTED":      "requested panel type is not supported",
		"CAPTURE_REPEAT_UNSUPPORTED":     "repeated panels are not supported",
		"CAPTURE_LAYOUT_UNSUPPORTED":     "dashboard layout is not supported",
		"CAPTURE_CONTEXT_CHANGED":        "dashboard inputs changed during capture",
		"CAPTURE_MODE_UNSUPPORTED":       "panel display mode is not supported",
		"CAPTURE_SVG_COMPLEXITY_LIMIT":   "SVG traversal exceeds the supported complexity",
		"CAPTURE_SVG_TEXT_LIMIT":         "SVG text exceeds the supported measurement budget",
		"CAPTURE_SVG_INVALID_GEOMETRY":   "SVG has invalid geometry",
		"CAPTURE_VALIDATION_LIMIT":       "SVG snapshot exceeds validation safety limits",
		"CAPTURE_PRODUCER_MISSING":       "no compatible SVG capture producer is available",
		"CAPTURE_PROTOCOL_UNSUPPORTED":   "SVG capture protocol is not supported",
		"CAPTURE_INSTANCE_AMBIGUOUS":     "more than one matching SVG panel instance is active",
		"CAPTURE_FRAME_UNSUPPORTED":      "SVG producer frame is not supported",
		"CAPTURE_DATA_STATE_UNSUPPORTED": "SVG panel data state is not supported",
		"CAPTURE_EXPORT_FAILED":          "SVG panel could not produce a snapshot",
		"CAPTURE_PAYLOAD_INVALID":        "SVG snapshot does not match the capture contract",
		"CAPTURE_PAYLOAD_TOO_LARGE":      "SVG snapshot exceeds the configured byte limit",
		"CAPTURE_TIMEOUT":                "SVG capture timed out",
	}
	message, ok := messages[code]
	if !ok {
		code, message = "CAPTURE_PAYLOAD_INVALID", messages["CAPTURE_PAYLOAD_INVALID"]
	}
	return Collection{Error: &Error{Code: code, Message: message}}
}

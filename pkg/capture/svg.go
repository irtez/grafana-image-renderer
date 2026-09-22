package capture

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/grafana/chromedp"
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
	_, err := page.AddScriptToEvaluateOnNewDocument(svgBootstrapScript(request.PanelID, maxBytes)).Do(ctx)
	return err
}

type svgScriptError struct {
	Code string `json:"code"`
}

type svgScriptResult struct {
	Status       string          `json:"status"`
	Identity     *SVGIdentity    `json:"identity"`
	Run          *SVGRun         `json:"run"`
	SnapshotJSON *string         `json:"snapshotJSON"`
	PayloadBytes int             `json:"payloadBytes"`
	Error        *svgScriptError `json:"error"`
}

// CDP переносит bounded JSON как строку: его собственное escaping не меняет
// byte counter снимка, и Go не преобразует исходные числовые токены через float64.
var svgTransportRead = `(() => {
  const state = ` + svgReadScript + `;
  if (state.status === 'terminal-ok') {
    return {status: state.status, identity: state.identity, run: state.run,
      snapshotJSON: JSON.stringify(state.snapshot), payloadBytes: state.payloadBytes};
  }
  return state;
})()`

func (*SVGCollector) Collect(ctx context.Context, request Request, maxBytes int) (Collection, error) {
	return collectSVG(ctx, request, maxBytes, func(ctx context.Context) (svgScriptResult, error) {
		var raw json.RawMessage
		if err := chromedp.Evaluate(svgTransportRead, &raw).Do(ctx); err != nil {
			return svgScriptResult{}, err
		}
		var state svgScriptResult
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&state); err != nil {
			return svgScriptResult{Status: "terminal-error", Error: &svgScriptError{Code: "CAPTURE_PAYLOAD_INVALID"}}, nil
		}
		return state, nil
	})
}

func collectSVG(ctx context.Context, request Request, maxBytes int, read func(context.Context) (svgScriptResult, error)) (Collection, error) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	lastStatus := ""
	for {
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return svgWaitError(lastStatus), nil
			}
			return Collection{}, err
		}
		state, err := read(ctx)
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return svgWaitError(lastStatus), nil
			}
			return Collection{}, err
		}
		if collection, terminal := svgCollectionFromScript(state, request, maxBytes); terminal {
			return collection, nil
		}
		lastStatus = state.Status
		select {
		case <-ctx.Done():
		case <-ticker.C:
		}
	}
}

func svgWaitError(status string) Collection {
	if status == "idle" {
		return svgError("CAPTURE_PRODUCER_MISSING")
	}
	return svgError("CAPTURE_TIMEOUT")
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
			return invalid()
		}
		return Collection{Payload: raw}, true
	default:
		return invalid()
	}
}

func svgError(code string) Collection {
	messages := map[string]string{
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

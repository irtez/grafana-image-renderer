package capture

import (
	"encoding/json"
	"fmt"
)

const ContractV2 = "siam-render-capture/v2"

type PanelResult struct {
	PanelID int             `json:"panelId"`
	Status  string          `json:"status"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}
type BatchCollection struct{ Panels []PanelResult }
type BatchMetadata struct {
	DashboardUID      string `json:"dashboardUid"`
	RequestedPanelIDs []int  `json:"requestedPanelIds"`
	From              string `json:"from"`
	To                string `json:"to"`
	Timezone          string `json:"timezone"`
	VariablesHash     string `json:"variablesHash"`
	StartedAtMs       int64  `json:"startedAtMs"`
	CapturedAtMs      int64  `json:"capturedAtMs"`
}
type BatchEnvelope struct {
	Contract string         `json:"contract"`
	Capture  *BatchMetadata `json:"capture"`
	Status   string         `json:"status"`
	Panels   []PanelResult  `json:"panels"`
	Error    *Error         `json:"error,omitempty"`
}

// MarshalBatch returns the encoded envelope and its final status, including any
// replacement caused by the response byte limit. The status is empty on error.
func MarshalBatch(request Request, panels []PanelResult, global *Error, startedAt, capturedAt int64, maxBytes int) ([]byte, string, error) {
	if maxBytes < 1 {
		return nil, "", fmt.Errorf("max JSON bytes must be positive")
	}
	envelope := BatchEnvelope{Contract: ContractV2, Status: "failed", Panels: panels, Error: global, Capture: &BatchMetadata{
		DashboardUID: request.DashboardUID, RequestedPanelIDs: request.PanelIDs, From: request.RenderFrom, To: request.RenderTo, Timezone: request.Timezone, VariablesHash: request.VariablesHash, StartedAtMs: startedAt, CapturedAtMs: capturedAt,
	}}
	failAll := func(err *Error) {
		envelope.Status = "failed"
		envelope.Error = err
		envelope.Panels = make([]PanelResult, len(request.PanelIDs))
		for i, id := range request.PanelIDs {
			envelope.Panels[i] = PanelResult{PanelID: id, Status: "error", Error: err}
		}
	}
	if global != nil {
		failAll(global)
	} else {
		if len(panels) != len(request.PanelIDs) || len(panels) == 0 {
			return nil, "", fmt.Errorf("batch must contain each requested panel")
		}
		ok := 0
		for i, p := range panels {
			if p.PanelID != request.PanelIDs[i] || (p.Status == "ok" && (p.Error != nil || len(p.Payload) == 0 || string(p.Payload) == "null")) || (p.Status == "error" && (p.Error == nil || len(p.Payload) != 0)) || (p.Status != "ok" && p.Status != "error") {
				return nil, "", fmt.Errorf("invalid batch panel entry")
			}
			if p.Status == "ok" {
				ok++
			}
		}
		if ok == len(panels) {
			envelope.Status = "complete"
		} else if ok > 0 {
			envelope.Status = "partial"
		}
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return nil, "", err
	}
	if len(body) <= maxBytes {
		return body, envelope.Status, nil
	}
	failAll(&Error{Code: "CAPTURE_PAYLOAD_TOO_LARGE", Message: "capture response exceeds the configured byte limit"})
	body, err = json.Marshal(envelope)
	if err != nil {
		return nil, "", err
	}
	if len(body) > maxBytes {
		return nil, "", fmt.Errorf("capture error envelope exceeds max JSON bytes")
	}
	return body, envelope.Status, nil
}

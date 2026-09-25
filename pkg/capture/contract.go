package capture

import (
	"encoding/json"
	"fmt"
)

const ContractV1 = "siam-render-capture/v1"

type Request struct {
	Version       int
	PanelIDs      []int
	DashboardUID  string
	PanelID       int
	Kind          string
	RenderFrom    string
	RenderTo      string
	Timezone      string
	VariablesHash string
}

type Metadata struct {
	DashboardUID  string `json:"dashboardUid"`
	PanelID       int    `json:"panelId"`
	Kind          string `json:"kind"`
	RenderFrom    string `json:"renderFrom,omitempty"`
	RenderTo      string `json:"renderTo,omitempty"`
	Timezone      string `json:"timezone,omitempty"`
	VariablesHash string `json:"variablesHash,omitempty"`
	CapturedAtMs  int64  `json:"capturedAtMs,omitempty"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Collection struct {
	Payload any
	Error   *Error
}

type PayloadTrimmer interface {
	TrimLast(reason string) bool
	OverflowError() Error
}

type successEnvelope struct {
	Contract string        `json:"contract"`
	Capture  Metadata      `json:"capture"`
	Result   successResult `json:"result"`
}

type successResult struct {
	Status  string `json:"status"`
	Payload any    `json:"payload"`
}

type errorEnvelope struct {
	Contract string      `json:"contract"`
	Capture  Metadata    `json:"capture"`
	Result   errorResult `json:"result"`
}

type errorResult struct {
	Status string `json:"status"`
	Error  Error  `json:"error"`
}

func MarshalCollection(meta Metadata, collection Collection, maxBytes int) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, fmt.Errorf("max JSON bytes must be positive")
	}
	if collection.Payload != nil && collection.Error != nil {
		return nil, fmt.Errorf("capture collection cannot contain both payload and error")
	}

	for {
		body, err := marshalEnvelope(meta, collection)
		if err != nil {
			return nil, fmt.Errorf("marshal capture envelope: %w", err)
		}
		if len(body) <= maxBytes {
			return body, nil
		}

		if collection.Error != nil {
			return nil, fmt.Errorf("capture error envelope exceeds max JSON bytes")
		}

		if trimmer, ok := collection.Payload.(PayloadTrimmer); ok {
			if trimmer.TrimLast("json_bytes") {
				continue
			}
			overflow := trimmer.OverflowError()
			collection = Collection{Error: &overflow}
			continue
		}

		collection = Collection{Error: &Error{
			Code:    "CAPTURE_PAYLOAD_TOO_LARGE",
			Message: "semantic capture payload exceeds the configured byte limit",
		}}
	}
}

func marshalEnvelope(meta Metadata, collection Collection) ([]byte, error) {
	if collection.Error != nil {
		return json.Marshal(errorEnvelope{
			Contract: ContractV1,
			Capture:  meta,
			Result: errorResult{
				Status: "error",
				Error:  *collection.Error,
			},
		})
	}

	return json.Marshal(successEnvelope{
		Contract: ContractV1,
		Capture:  meta,
		Result: successResult{
			Status:  "ok",
			Payload: collection.Payload,
		},
	})
}

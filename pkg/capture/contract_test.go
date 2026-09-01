package capture

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type literalPayload struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type trimmingPayload struct {
	Kind string   `json:"kind"`
	Rows []string `json:"rows"`
}

func (p *trimmingPayload) TrimLast(_ string) bool {
	if len(p.Rows) <= 1 {
		return false
	}
	p.Rows = p.Rows[:len(p.Rows)-1]
	return true
}

func (p *trimmingPayload) OverflowError() Error {
	return Error{Code: "TABLE_ROW_TOO_LARGE", Message: "row 0 does not fit an empty semantic capture"}
}

func contractMetadata() Metadata {
	return Metadata{
		DashboardUID:  "dash",
		PanelID:       4,
		Kind:          "grafana-table",
		RenderFrom:    "100",
		RenderTo:      "200",
		Timezone:      "Europe/Moscow",
		VariablesHash: "sha256:abc",
		CapturedAtMs:  201,
	}
}

func TestMarshalCollectionSuccessContract(t *testing.T) {
	body, err := MarshalCollection(contractMetadata(), Collection{
		Payload: literalPayload{Kind: "literal", Value: "ok"},
	}, 4096)

	require.NoError(t, err)
	require.JSONEq(t, `{
  "contract":"siam-render-capture/v1",
  "capture":{"dashboardUid":"dash","panelId":4,"kind":"grafana-table",
    "renderFrom":"100","renderTo":"200","timezone":"Europe/Moscow",
    "variablesHash":"sha256:abc","capturedAtMs":201},
  "result":{"status":"ok","payload":{"kind":"literal","value":"ok"}}
}`, string(body))
	require.NotContains(t, string(body), `"error"`)
}

func TestMarshalCollectionErrorContract(t *testing.T) {
	body, err := MarshalCollection(contractMetadata(), Collection{
		Error: &Error{Code: "TABLE_QUERY_ERROR", Message: "table query failed"},
	}, 4096)

	require.NoError(t, err)
	require.JSONEq(t, `{
  "contract":"siam-render-capture/v1",
  "capture":{"dashboardUid":"dash","panelId":4,"kind":"grafana-table",
    "renderFrom":"100","renderTo":"200","timezone":"Europe/Moscow",
    "variablesHash":"sha256:abc","capturedAtMs":201},
  "result":{"status":"error","error":{"code":"TABLE_QUERY_ERROR","message":"table query failed"}}
}`, string(body))
	require.NotContains(t, string(body), `"payload"`)
}

func TestMarshalCollectionRemovesWholeRowsUntilBodyFits(t *testing.T) {
	payload := &trimmingPayload{Kind: "literal", Rows: []string{
		strings.Repeat("a", 80),
		strings.Repeat("b", 80),
		strings.Repeat("c", 80),
	}}
	wantPayload := &trimmingPayload{Kind: "literal", Rows: []string{
		strings.Repeat("a", 80),
		strings.Repeat("b", 80),
	}}
	wantBody, err := MarshalCollection(contractMetadata(), Collection{Payload: wantPayload}, 4096)
	require.NoError(t, err)

	body, err := MarshalCollection(contractMetadata(), Collection{Payload: payload}, len(wantBody))

	require.NoError(t, err)
	require.LessOrEqual(t, len(body), len(wantBody))
	require.JSONEq(t, string(wantBody), string(body))
	require.Equal(t, wantPayload.Rows, payload.Rows)
}

func TestMarshalCollectionReturnsBoundedOverflowWithoutRowContent(t *testing.T) {
	secret := strings.Repeat("secret-cell", 30)
	payload := &trimmingPayload{Kind: "literal", Rows: []string{secret}}

	body, err := MarshalCollection(contractMetadata(), Collection{Payload: payload}, 512)

	require.NoError(t, err)
	require.LessOrEqual(t, len(body), 512)
	require.Contains(t, string(body), `"code":"TABLE_ROW_TOO_LARGE"`)
	require.NotContains(t, string(body), secret)
	require.NotContains(t, string(body), `"payload"`)
}

func TestMarshalCollectionUsesGenericOverflowForNonTrimmablePayload(t *testing.T) {
	body, err := MarshalCollection(contractMetadata(), Collection{
		Payload: literalPayload{Kind: "literal", Value: strings.Repeat("x", 1000)},
	}, 512)

	require.NoError(t, err)
	require.Contains(t, string(body), `"code":"CAPTURE_PAYLOAD_TOO_LARGE"`)
	require.NotContains(t, string(body), strings.Repeat("x", 20))
}

func TestMarshalCollectionRejectsInvalidOrImpossibleLimit(t *testing.T) {
	_, err := MarshalCollection(contractMetadata(), Collection{Payload: literalPayload{}}, 0)
	require.ErrorContains(t, err, "max JSON bytes must be positive")

	_, err = MarshalCollection(contractMetadata(), Collection{Payload: literalPayload{}}, 1)
	require.ErrorContains(t, err, "capture error envelope exceeds max JSON bytes")
}

package capture

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func validTablePayload() *TablePayload {
	return &TablePayload{
		Kind:          "grafana-table",
		SchemaVersion: 1,
		Panel:         TablePanel{ID: 4, Title: "Services"},
		Observed: TableObserved{
			DataState:       "Done",
			EffectiveFromMs: 100,
			EffectiveToMs:   200,
		},
		Frame:   TableFrame{Index: 0, Name: "A", TotalFrames: 1},
		Columns: []string{"Service", "Value"},
		Dataset: TableDataset{TotalRows: 3, CapturedRows: 3, Complete: true},
		Rows: []TableRow{
			{Position: 0, Cells: []string{"gamma", "75%"}},
			{Position: 1, Cells: []string{"beta", "50%"}},
			{Position: 2, Cells: []string{"alpha", "25%"}},
		},
	}
}

func TestTablePayloadTrimsOnlyTheLastWholeRow(t *testing.T) {
	payload := validTablePayload()

	require.True(t, payload.TrimLast("json_bytes"))

	require.Equal(t, 2, payload.Dataset.CapturedRows)
	require.False(t, payload.Dataset.Complete)
	require.NotNil(t, payload.Dataset.LimitedBy)
	require.Equal(t, "json_bytes", *payload.Dataset.LimitedBy)
	require.Equal(t, []TableRow{
		{Position: 0, Cells: []string{"gamma", "75%"}},
		{Position: 1, Cells: []string{"beta", "50%"}},
	}, payload.Rows)
}

func TestTablePayloadKeepsOneRowForOverflowDiagnosis(t *testing.T) {
	secret := strings.Repeat("secret-value", 20)
	payload := validTablePayload()
	payload.Dataset = TableDataset{TotalRows: 1, CapturedRows: 1, Complete: true}
	payload.Rows = []TableRow{{Position: 0, Cells: []string{secret, "75%"}}}

	require.False(t, payload.TrimLast("json_bytes"))
	overflow := payload.OverflowError()

	require.Equal(t, "TABLE_ROW_TOO_LARGE", overflow.Code)
	require.Contains(t, overflow.Message, "row 0")
	require.Contains(t, overflow.Message, "UTF-8 bytes")
	require.NotContains(t, overflow.Message, secret)
}

func TestValidateTablePayloadAcceptsExactContract(t *testing.T) {
	require.NoError(t, validateTablePayload(validTablePayload(), Request{PanelID: 4}))
}

func TestValidateTablePayloadRejectsBrokenInvariants(t *testing.T) {
	jsonBytes := "json_bytes"
	tests := []struct {
		name   string
		mutate func(*TablePayload)
	}{
		{name: "wrong kind", mutate: func(p *TablePayload) { p.Kind = "alert-list" }},
		{name: "wrong schema", mutate: func(p *TablePayload) { p.SchemaVersion = 2 }},
		{name: "wrong panel", mutate: func(p *TablePayload) { p.Panel.ID = 5 }},
		{name: "nonterminal data", mutate: func(p *TablePayload) { p.Observed.DataState = "Loading" }},
		{name: "reversed time", mutate: func(p *TablePayload) { p.Observed.EffectiveFromMs = 300 }},
		{name: "missing columns", mutate: func(p *TablePayload) { p.Columns = nil }},
		{name: "missing frame", mutate: func(p *TablePayload) { p.Frame.TotalFrames = 0 }},
		{name: "frame out of range", mutate: func(p *TablePayload) { p.Frame.Index = 1 }},
		{name: "total smaller than captured", mutate: func(p *TablePayload) { p.Dataset.TotalRows = 2 }},
		{name: "captured count mismatch", mutate: func(p *TablePayload) { p.Dataset.CapturedRows = 2 }},
		{name: "complete mismatch", mutate: func(p *TablePayload) { p.Dataset.Complete = false }},
		{name: "limited complete", mutate: func(p *TablePayload) { p.Dataset.LimitedBy = &jsonBytes }},
		{name: "unknown limit", mutate: func(p *TablePayload) { other := "rows"; p.Dataset.LimitedBy = &other; p.Dataset.Complete = false }},
		{name: "gapped positions", mutate: func(p *TablePayload) { p.Rows[1].Position = 2 }},
		{name: "wrong cell count", mutate: func(p *TablePayload) { p.Rows[1].Cells = []string{"beta"} }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := validTablePayload()
			tt.mutate(payload)

			err := validateTablePayload(payload, Request{PanelID: 4})

			require.Error(t, err)
			require.NotContains(t, err.Error(), "gamma")
			require.NotContains(t, err.Error(), "beta")
		})
	}
}

func TestValidateTablePayloadAcceptsJSONLimitedPrefix(t *testing.T) {
	payload := validTablePayload()
	limitedBy := "json_bytes"
	payload.Dataset = TableDataset{TotalRows: 3, CapturedRows: 2, Complete: false, LimitedBy: &limitedBy}
	payload.Rows = payload.Rows[:2]

	require.NoError(t, validateTablePayload(payload, Request{PanelID: 4}))
}

func TestTableScriptResultMapsOnlyAllowlistedDomainErrors(t *testing.T) {
	codes := []string{
		"TABLE_PANEL_NOT_FOUND",
		"TABLE_PANEL_TYPE_MISMATCH",
		"TABLE_DATA_NOT_READY",
		"TABLE_QUERY_ERROR",
		"TABLE_FRAME_MISSING",
		"TABLE_NESTED_FRAME_UNSUPPORTED",
		"TABLE_GRID_NOT_FOUND",
		"TABLE_GRID_INCONSISTENT",
		"TABLE_PAGINATION_STALLED",
		"TABLE_ROW_TOO_LARGE",
	}
	for _, code := range codes {
		t.Run(code, func(t *testing.T) {
			scriptError := &tableScriptError{Code: code}
			if code == "TABLE_ROW_TOO_LARGE" {
				position, rowBytes := 0, 1024
				scriptError.Position = &position
				scriptError.RowUTF8Bytes = &rowBytes
			}
			collection, err := tableCollectionFromScript(tableScriptResult{
				Error: scriptError,
			}, Request{PanelID: 4})

			require.NoError(t, err)
			require.NotNil(t, collection.Error)
			require.Equal(t, code, collection.Error.Code)
		})
	}

	_, err := tableCollectionFromScript(tableScriptResult{
		Error: &tableScriptError{Code: "DATASOURCE_SECRET"},
	}, Request{PanelID: 4})
	require.ErrorContains(t, err, "unknown table capture error code")
}

func TestTableScriptResultRejectsAmbiguousOrMissingResult(t *testing.T) {
	payload := validTablePayload()

	_, err := tableCollectionFromScript(tableScriptResult{}, Request{PanelID: 4})
	require.ErrorContains(t, err, "exactly one")

	_, err = tableCollectionFromScript(tableScriptResult{
		Payload: payload,
		Error:   &tableScriptError{Code: "TABLE_QUERY_ERROR"},
	}, Request{PanelID: 4})
	require.ErrorContains(t, err, "exactly one")
}

func TestTableCollectorKind(t *testing.T) {
	require.Equal(t, "grafana-table", NewTableCollector().Kind())
}

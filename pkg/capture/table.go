package capture

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/chromedp/cdproto/runtime"
	"github.com/grafana/chromedp"
)

type TablePanel struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

type TableObserved struct {
	DataState       string `json:"dataState"`
	EffectiveFromMs int64  `json:"effectiveFromMs"`
	EffectiveToMs   int64  `json:"effectiveToMs"`
}

type TableFrame struct {
	Index       int    `json:"index"`
	Name        string `json:"name"`
	TotalFrames int    `json:"totalFrames"`
}

type TableDataset struct {
	TotalRows    int     `json:"totalRows"`
	CapturedRows int     `json:"capturedRows"`
	Complete     bool    `json:"complete"`
	LimitedBy    *string `json:"limitedBy"`
}

type TableRow struct {
	Position int      `json:"position"`
	Cells    []string `json:"cells"`
}

type TablePayload struct {
	Kind          string        `json:"kind"`
	SchemaVersion int           `json:"schemaVersion"`
	Panel         TablePanel    `json:"panel"`
	Observed      TableObserved `json:"observed"`
	Frame         TableFrame    `json:"frame"`
	Columns       []string      `json:"columns"`
	Dataset       TableDataset  `json:"dataset"`
	Rows          []TableRow    `json:"rows"`
}

func (p *TablePayload) TrimLast(reason string) bool {
	if len(p.Rows) <= 1 {
		return false
	}
	p.Rows = p.Rows[:len(p.Rows)-1]
	p.Dataset.CapturedRows = len(p.Rows)
	p.Dataset.Complete = false
	p.Dataset.LimitedBy = &reason
	return true
}

func (p *TablePayload) OverflowError() Error {
	if len(p.Rows) == 0 {
		return Error{
			Code:    "CAPTURE_PAYLOAD_TOO_LARGE",
			Message: "semantic capture metadata exceeds the configured byte limit",
		}
	}
	row := p.Rows[0]
	encoded, err := json.Marshal(row)
	if err != nil {
		return Error{Code: "TABLE_ROW_TOO_LARGE", Message: fmt.Sprintf("row %d does not fit an empty semantic capture", row.Position)}
	}
	return Error{
		Code:    "TABLE_ROW_TOO_LARGE",
		Message: fmt.Sprintf("row %d (%d UTF-8 bytes) does not fit an empty semantic capture", row.Position, len(encoded)),
	}
}

type tableScriptError struct {
	Code         string `json:"code"`
	Position     *int   `json:"position,omitempty"`
	RowUTF8Bytes *int   `json:"rowUtf8Bytes,omitempty"`
}

type tableScriptResult struct {
	Payload *TablePayload     `json:"payload,omitempty"`
	Error   *tableScriptError `json:"error,omitempty"`
}

type tableScriptArgs struct {
	PanelID      int `json:"panelId"`
	MaxJSONBytes int `json:"maxJSONBytes"`
}

type TableCollector struct{}

func NewTableCollector() *TableCollector {
	return &TableCollector{}
}

func (*TableCollector) Kind() string {
	return "grafana-table"
}

func (*TableCollector) Collect(ctx context.Context, request Request, maxJSONBytes int) (Collection, error) {
	args, err := json.Marshal(tableScriptArgs{PanelID: request.PanelID, MaxJSONBytes: maxJSONBytes})
	if err != nil {
		return Collection{}, fmt.Errorf("marshal table capture arguments: %w", err)
	}
	expression := fmt.Sprintf("(%s)(JSON.parse(%s))", tableCaptureScript, strconv.Quote(string(args)))

	var result tableScriptResult
	err = chromedp.Evaluate(expression, &result, func(params *runtime.EvaluateParams) *runtime.EvaluateParams {
		return params.WithAwaitPromise(true)
	}).Do(ctx)
	if err != nil {
		return Collection{}, fmt.Errorf("evaluate table capture: %w", err)
	}

	return tableCollectionFromScript(result, request)
}

func tableCollectionFromScript(result tableScriptResult, request Request) (Collection, error) {
	if (result.Payload == nil) == (result.Error == nil) {
		return Collection{}, fmt.Errorf("table capture script must return exactly one payload or error")
	}
	if result.Error != nil {
		message, err := tableErrorMessage(*result.Error)
		if err != nil {
			return Collection{}, err
		}
		return Collection{Error: &Error{Code: result.Error.Code, Message: message}}, nil
	}
	if err := validateTablePayload(result.Payload, request); err != nil {
		return Collection{}, fmt.Errorf("invalid table capture payload: %w", err)
	}
	return Collection{Payload: result.Payload}, nil
}

func tableErrorMessage(scriptError tableScriptError) (string, error) {
	messages := map[string]string{
		"TABLE_PANEL_NOT_FOUND":          "table panel was not found in the rendered dashboard",
		"TABLE_PANEL_TYPE_MISMATCH":      "requested panel is not a table",
		"TABLE_DATA_NOT_READY":           "table data is not ready",
		"TABLE_QUERY_ERROR":              "table query failed",
		"TABLE_FRAME_MISSING":            "selected table frame is missing",
		"TABLE_NESTED_FRAME_UNSUPPORTED": "nested table frames are not supported",
		"TABLE_GRID_NOT_FOUND":           "rendered TableNG grid was not found",
		"TABLE_GRID_INCONSISTENT":        "rendered TableNG grid is inconsistent",
		"TABLE_PAGINATION_STALLED":       "rendered TableNG pagination stalled",
	}
	if message, ok := messages[scriptError.Code]; ok {
		return message, nil
	}
	if scriptError.Code == "TABLE_ROW_TOO_LARGE" {
		if scriptError.Position == nil || scriptError.RowUTF8Bytes == nil || *scriptError.Position < 0 || *scriptError.RowUTF8Bytes < 0 {
			return "", fmt.Errorf("invalid TABLE_ROW_TOO_LARGE details")
		}
		return fmt.Sprintf("row %d (%d UTF-8 bytes) does not fit an empty semantic capture", *scriptError.Position, *scriptError.RowUTF8Bytes), nil
	}
	return "", fmt.Errorf("unknown table capture error code %q", scriptError.Code)
}

func validateTablePayload(payload *TablePayload, request Request) error {
	if payload == nil {
		return fmt.Errorf("payload is nil")
	}
	if payload.Kind != "grafana-table" {
		return fmt.Errorf("unexpected payload kind")
	}
	if payload.SchemaVersion != 1 {
		return fmt.Errorf("unexpected schema version")
	}
	if payload.Panel.ID != request.PanelID {
		return fmt.Errorf("panel ID does not match request")
	}
	if payload.Observed.DataState != "Done" {
		return fmt.Errorf("data state is not terminal")
	}
	if payload.Observed.EffectiveFromMs > payload.Observed.EffectiveToMs {
		return fmt.Errorf("effective time range is reversed")
	}
	if payload.Frame.TotalFrames <= 0 || payload.Frame.Index < 0 || payload.Frame.Index >= payload.Frame.TotalFrames {
		return fmt.Errorf("selected frame is invalid")
	}
	if payload.Columns == nil || len(payload.Columns) == 0 {
		return fmt.Errorf("columns are missing")
	}
	if payload.Rows == nil {
		return fmt.Errorf("rows are missing")
	}
	if payload.Dataset.TotalRows < 0 || payload.Dataset.CapturedRows < 0 || payload.Dataset.CapturedRows > payload.Dataset.TotalRows {
		return fmt.Errorf("dataset row counts are invalid")
	}
	if payload.Dataset.CapturedRows != len(payload.Rows) {
		return fmt.Errorf("captured row count does not match rows")
	}
	if payload.Dataset.LimitedBy == nil {
		if payload.Dataset.Complete != (payload.Dataset.CapturedRows == payload.Dataset.TotalRows) {
			return fmt.Errorf("dataset completeness is inconsistent")
		}
	} else if *payload.Dataset.LimitedBy != "json_bytes" || payload.Dataset.Complete || payload.Dataset.CapturedRows >= payload.Dataset.TotalRows {
		return fmt.Errorf("dataset byte limit is inconsistent")
	}
	for index, row := range payload.Rows {
		if row.Position != index {
			return fmt.Errorf("row positions are not contiguous")
		}
		if row.Cells == nil || len(row.Cells) != len(payload.Columns) {
			return fmt.Errorf("row %d has an invalid cell count", index)
		}
	}
	return nil
}

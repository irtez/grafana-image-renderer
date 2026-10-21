package capture

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCollectSVGBatchEmptyPanelIDs(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	got, err := collectSVGBatch(ctx, Request{PanelIDs: []int{}}, 4194304, nil)
	require.NoError(t, err)
	require.NotNil(t, got.Error)
	require.Equal(t, "CAPTURE_PAYLOAD_INVALID", got.Error.Code)
}

func TestCollectSVGBatchZeroMaxBytes(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	got, err := collectSVGBatch(ctx, Request{PanelIDs: []int{1}}, 0, nil)
	require.NoError(t, err)
	require.NotNil(t, got.Error)
	require.Equal(t, "CAPTURE_PAYLOAD_INVALID", got.Error.Code)
}

func TestCollectSVGBatchNegativeMaxBytes(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	got, err := collectSVGBatch(ctx, Request{PanelIDs: []int{1}}, -1, nil)
	require.NoError(t, err)
	require.NotNil(t, got.Error)
	require.Equal(t, "CAPTURE_PAYLOAD_INVALID", got.Error.Code)
}

func TestCollectSVGBatchLayoutError(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	got, err := collectSVGBatch(ctx, batchRequest(), 4194304, func(context.Context, int, map[int]svgStamp) (svgBatchStep, error) {
		return svgBatchStep{Layout: svgLayoutResult{Status: "error", Error: &svgScriptError{Code: "CAPTURE_EXPORT_FAILED"}}}, nil
	})
	require.NoError(t, err)
	require.NotNil(t, got.Error)
	require.Equal(t, "CAPTURE_EXPORT_FAILED", got.Error.Code)
}

func TestCollectSVGBatchBatchStepError(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	got, err := collectSVGBatch(ctx, batchRequest(), 4194304, func(context.Context, int, map[int]svgStamp) (svgBatchStep, error) {
		return svgBatchStep{Error: &svgScriptError{Code: "CAPTURE_DATA_STATE_UNSUPPORTED"}}, nil
	})
	require.NoError(t, err)
	require.NotNil(t, got.Error)
	require.Equal(t, "CAPTURE_DATA_STATE_UNSUPPORTED", got.Error.Code)
}

func TestCollectSVGBatchReaderError(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	got, err := collectSVGBatch(ctx, batchRequest(), 4194304, func(context.Context, int, map[int]svgStamp) (svgBatchStep, error) {
		return svgBatchStep{}, fmt.Errorf("reader error")
	})
	require.Error(t, err)
	require.Nil(t, got.Payload)
}

func TestCollectSVGBatchInvalidReadyLayout(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	got, err := collectSVGBatch(ctx, batchRequest(), 4194304, func(context.Context, int, map[int]svgStamp) (svgBatchStep, error) {
		return svgBatchStep{
			Layout: svgLayoutResult{Status: "ready", Panels: []svgLayoutPanel{{PanelID: 8, Status: "ready", Active: true}}},
			States: []svgPanelState{{PanelID: 8, State: svgScriptResult{Status: "pending"}}},
		}, nil
	})
	require.NoError(t, err)
	require.NotNil(t, got.Error)
	require.Equal(t, "CAPTURE_PAYLOAD_INVALID", got.Error.Code)
}

func TestCollectSVGBatchInvalidReadyPanelIDs(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	got, err := collectSVGBatch(ctx, batchRequest(), 4194304, func(context.Context, int, map[int]svgStamp) (svgBatchStep, error) {
		return svgBatchStep{
			Layout: svgLayoutResult{Status: "ready", ContextKey: "ctx", Panels: []svgLayoutPanel{{PanelID: 99, Status: "ready", Active: true}}},
			States: []svgPanelState{{PanelID: 99, State: svgScriptResult{Status: "pending"}}},
		}, nil
	})
	require.NoError(t, err)
	require.NotNil(t, got.Error)
	require.Equal(t, "CAPTURE_PAYLOAD_INVALID", got.Error.Code)
}

func TestCollectSVGBatchInvalidLayoutStatus(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	got, err := collectSVGBatch(ctx, batchRequest(), 4194304, func(context.Context, int, map[int]svgStamp) (svgBatchStep, error) {
		return svgBatchStep{Layout: svgLayoutResult{Status: "failed"}}, nil
	})
	require.NoError(t, err)
	require.NotNil(t, got.Error)
	require.Equal(t, "CAPTURE_PAYLOAD_INVALID", got.Error.Code)
}

func TestSVGBatchReadScriptWithStamps(t *testing.T) {
	t.Parallel()
	request := batchRequest()
	request.MaxResponseBytes = 1000000
	stamps := map[int]svgStamp{8: {Identity: &SVGIdentity{ProducerID: "test", PanelID: 8}, Run: &SVGRun{Generation: 1}, PayloadBytes: 100}}
	script := svgBatchReadScript(request, 8, stamps)
	require.Contains(t, script, svgLayoutScript)
	require.Contains(t, script, svgReadScript)
	require.Contains(t, script, "focusID")
	require.Contains(t, script, "maxTotalBytes")
	require.Contains(t, script, "1000000")
	require.Contains(t, script, "100")
}

func TestSVGBatchReadScriptWithDefaultMaxResponseBytes(t *testing.T) {
	t.Parallel()
	request := batchRequest()
	request.MaxResponseBytes = 0
	stamps := map[int]svgStamp{}
	script := svgBatchReadScript(request, -1, stamps)
	require.Contains(t, script, "16777216")
}

func TestSVGBatchReadScriptWithEmptyStamps(t *testing.T) {
	t.Parallel()
	request := batchRequest()
	script := svgBatchReadScript(request, -1, nil)
	require.Contains(t, script, svgLayoutScript)
	require.Contains(t, script, svgReadScript)
	require.Contains(t, script, "stamps")
}

func TestSameSVGStampWithNilValues(t *testing.T) {
	t.Parallel()
	var stamp svgStamp
	result := sameSVGStamp(stamp, svgScriptResult{Status: "terminal-ok", Identity: nil, Run: nil})
	require.False(t, result)
}

func TestSameSVGStampMatching(t *testing.T) {
	t.Parallel()
	identity := SVGIdentity{ProducerID: "p", ProducerVersion: "1", PanelID: 7, InstanceID: "i"}
	run := SVGRun{Generation: 1, EffectiveFromMs: 100, EffectiveToMs: 200}
	stamp := svgStamp{Identity: &identity, Run: &run, PayloadBytes: 1000}
	result := sameSVGStamp(stamp, svgScriptResult{Status: "terminal-ok", Identity: &identity, Run: &run, PayloadBytes: 1000})
	require.True(t, result)
}

func TestSameSVGStampDifferentPayloadBytes(t *testing.T) {
	t.Parallel()
	identity := SVGIdentity{ProducerID: "p", ProducerVersion: "1", PanelID: 7, InstanceID: "i"}
	run := SVGRun{Generation: 1, EffectiveFromMs: 100, EffectiveToMs: 200}
	stamp := svgStamp{Identity: &identity, Run: &run, PayloadBytes: 1000}
	result := sameSVGStamp(stamp, svgScriptResult{Status: "terminal-ok", Identity: &identity, Run: &run, PayloadBytes: 2000})
	require.False(t, result)
}

func TestSameSVGStampDifferentIdentity(t *testing.T) {
	t.Parallel()
	identity := SVGIdentity{ProducerID: "p", ProducerVersion: "1", PanelID: 7, InstanceID: "i"}
	identity2 := SVGIdentity{ProducerID: "p2", ProducerVersion: "1", PanelID: 7, InstanceID: "i"}
	run := SVGRun{Generation: 1, EffectiveFromMs: 100, EffectiveToMs: 200}
	stamp := svgStamp{Identity: &identity, Run: &run, PayloadBytes: 1000}
	result := sameSVGStamp(stamp, svgScriptResult{Status: "terminal-ok", Identity: &identity2, Run: &run, PayloadBytes: 1000})
	require.False(t, result)
}

func TestCollectSVGBatchPanelErrorWithResult(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	got, err := collectSVGBatch(ctx, batchRequest(), 4194304, func(context.Context, int, map[int]svgStamp) (svgBatchStep, error) {
		return svgBatchStep{
			Layout: svgLayoutResult{Status: "ready", ContextKey: "ctx", Panels: []svgLayoutPanel{{PanelID: 8, Status: "error", Active: true, Error: &svgScriptError{Code: "CAPTURE_EXPORT_FAILED"}}, {PanelID: 7, Status: "ready", Active: true}}},
			States: []svgPanelState{{PanelID: 8, State: svgScriptResult{Status: "pending"}}, {PanelID: 7, State: svgScriptResult{Status: "pending"}}},
		}, nil
	})
	require.NoError(t, err)
	batch, ok := got.Payload.(BatchCollection)
	require.True(t, ok)
	require.Equal(t, "error", batch.Panels[0].Status)
	require.Equal(t, "CAPTURE_EXPORT_FAILED", batch.Panels[0].Error.Code)
}

func TestCollectSVGBatchMismatchedStatePanelID(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	got, err := collectSVGBatch(ctx, batchRequest(), 4194304, func(context.Context, int, map[int]svgStamp) (svgBatchStep, error) {
		return svgBatchStep{
			Layout: svgLayoutResult{Status: "ready", ContextKey: "ctx", Panels: []svgLayoutPanel{{PanelID: 8, Status: "ready", Active: true}, {PanelID: 7, Status: "ready", Active: true}}},
			States: []svgPanelState{{PanelID: 8, State: svgScriptResult{Status: "pending"}}, {PanelID: 99, State: svgScriptResult{Status: "pending"}}},
		}, nil
	})
	require.NoError(t, err)
	require.NotNil(t, got.Error)
	require.Equal(t, "CAPTURE_PAYLOAD_INVALID", got.Error.Code)
}

func TestCollectSVGBatchPendingStatusInvalid(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	got, err := collectSVGBatch(ctx, batchRequest(), 4194304, func(context.Context, int, map[int]svgStamp) (svgBatchStep, error) {
		return svgBatchStep{
			Layout: svgLayoutResult{Status: "ready", ContextKey: "ctx", Panels: []svgLayoutPanel{{PanelID: 8, Status: "pending", Active: true}, {PanelID: 7, Status: "ready", Active: true}}},
			States: []svgPanelState{{PanelID: 8, State: svgScriptResult{Status: "pending"}}, {PanelID: 7, State: svgScriptResult{Status: "pending"}}},
		}, nil
	})
	require.NoError(t, err)
	require.NotNil(t, got.Error)
	require.Equal(t, "CAPTURE_PAYLOAD_INVALID", got.Error.Code)
}

func TestCollectSVGBatchDataPendingWithTerminalError(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	got, err := collectSVGBatch(ctx, batchRequest(), 4194304, func(context.Context, int, map[int]svgStamp) (svgBatchStep, error) {
		return svgBatchStep{
			Layout: svgLayoutResult{Status: "ready", ContextKey: "ctx", Panels: []svgLayoutPanel{{PanelID: 8, Status: "ready", Active: true, DataPending: true}, {PanelID: 7, Status: "ready", Active: true}}},
			States: []svgPanelState{{PanelID: 8, State: svgScriptResult{Status: "terminal-error"}}, {PanelID: 7, State: svgScriptResult{Status: "pending"}}},
		}, nil
	})
	require.NoError(t, err)
	batch, ok := got.Payload.(BatchCollection)
	require.True(t, ok)
	require.Equal(t, "error", batch.Panels[1].Status)
}

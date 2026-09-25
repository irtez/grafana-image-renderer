package capture

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func batchFixture(t *testing.T, id int) svgScriptResult {
	state, _ := svgTransportFixture(t)
	v := decodeSVGFixture(t, []byte(*state.SnapshotJSON))
	svgFixtureSet(v, "panel/id", id)
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	body := string(raw)
	state.SnapshotJSON = &body
	state.PayloadBytes = len(raw)
	state.Identity.PanelID = id
	return state
}
func batchStep(states ...svgPanelState) svgBatchStep {
	panels := make([]svgLayoutPanel, len(states))
	for i, s := range states {
		panels[i] = svgLayoutPanel{PanelID: s.PanelID, Status: "ready", Active: true}
	}
	return svgBatchStep{Layout: svgLayoutResult{Status: "ready", ContextKey: "context-a", Panels: panels}, States: states}
}
func TestSVGBatchRetainsSnapshotBeforeUnmountAndKeepsRequestedOrder(t *testing.T) {
	request := batchRequest()
	calls := 0
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	got, err := collectSVGBatch(ctx, request, 4194304, func(context.Context, int, map[int]svgStamp) (svgBatchStep, error) {
		calls++
		if calls == 1 {
			return batchStep(svgPanelState{8, batchFixture(t, 8)}, svgPanelState{7, svgScriptResult{Status: "pending"}}), nil
		}
		next := batchStep(svgPanelState{8, svgScriptResult{Status: "idle"}}, svgPanelState{7, batchFixture(t, 7)})
		next.Layout.Panels[0].Active = false
		return next, nil
	})
	require.NoError(t, err)
	require.Nil(t, got.Error)
	batch, ok := got.Payload.(BatchCollection)
	require.True(t, ok)
	require.Equal(t, []int{8, 7}, []int{batch.Panels[0].PanelID, batch.Panels[1].PanelID})
	require.Equal(t, "ok", batch.Panels[0].Status)
	require.Equal(t, "ok", batch.Panels[1].Status)
}
func TestSVGBatchDeadlinePreservesReadySibling(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 80*time.Millisecond)
	defer cancel()
	start := time.Now()
	got, err := collectSVGBatch(ctx, batchRequest(), 4194304, func(context.Context, int, map[int]svgStamp) (svgBatchStep, error) {
		return batchStep(svgPanelState{8, batchFixture(t, 8)}, svgPanelState{7, svgScriptResult{Status: "pending"}}), nil
	})
	require.NoError(t, err)
	batch, ok := got.Payload.(BatchCollection)
	require.True(t, ok)
	require.Equal(t, "ok", batch.Panels[0].Status)
	require.Equal(t, "CAPTURE_TIMEOUT", batch.Panels[1].Error.Code)
	require.Less(t, time.Since(start), 300*time.Millisecond)
}
func TestSVGBatchNewRunInvalidatesRetainedSnapshot(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	calls := 0
	got, err := collectSVGBatch(ctx, batchRequest(), 4194304, func(context.Context, int, map[int]svgStamp) (svgBatchStep, error) {
		calls++
		if calls == 1 {
			return batchStep(svgPanelState{8, batchFixture(t, 8)}, svgPanelState{7, svgScriptResult{Status: "pending"}}), nil
		}
		return batchStep(svgPanelState{8, svgScriptResult{Status: "pending"}}, svgPanelState{7, batchFixture(t, 7)}), nil
	})
	require.NoError(t, err)
	batch, ok := got.Payload.(BatchCollection)
	require.True(t, ok)
	require.Empty(t, batch.Panels[0].Payload)
	require.Equal(t, "CAPTURE_TIMEOUT", batch.Panels[0].Error.Code)
	require.Equal(t, "ok", batch.Panels[1].Status)
}
func TestSVGBatchContextChangeRejectsMixedResults(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	calls := 0
	got, err := collectSVGBatch(ctx, batchRequest(), 4194304, func(context.Context, int, map[int]svgStamp) (svgBatchStep, error) {
		calls++
		step := batchStep(svgPanelState{8, batchFixture(t, 8)}, svgPanelState{7, svgScriptResult{Status: "pending"}})
		if calls > 1 {
			step.Layout.ContextKey = "context-b"
		}
		return step, nil
	})
	require.NoError(t, err)
	require.Nil(t, got.Payload)
	require.NotNil(t, got.Error)
	require.Equal(t, "CAPTURE_CONTEXT_CHANGED", got.Error.Code)
}
func TestSVGBatchDoesNotConvertClientCancellationToSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := collectSVGBatch(ctx, batchRequest(), 4194304, nil)
	require.ErrorIs(t, err, context.Canceled)
}

func TestSVGBatchCacheStampMatchesBrowserKeys(t *testing.T) {
	state := batchFixture(t, 7)
	raw, err := json.Marshal(svgStamp{Identity: state.Identity, Run: state.Run, PayloadBytes: state.PayloadBytes})
	require.NoError(t, err)
	var stamp map[string]any
	require.NoError(t, json.Unmarshal(raw, &stamp))
	require.Contains(t, stamp, "identity")
	require.Contains(t, stamp, "run")
	require.Contains(t, stamp, "payloadBytes")
}

func TestSVGBatchDeadlineDistinguishesMissingPendingAndInactive(t *testing.T) {
	for _, tc := range []struct {
		status string
		active bool
		code   string
	}{
		{"idle", true, "CAPTURE_PRODUCER_MISSING"}, {"pending", true, "CAPTURE_TIMEOUT"}, {"idle", false, "CAPTURE_TIMEOUT"},
	} {
		t.Run(tc.status+tc.code, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 75*time.Millisecond)
			defer cancel()
			calls := 0
			got, err := collectSVGBatch(ctx, batchRequest(), 4194304, func(ctx context.Context, _ int, _ map[int]svgStamp) (svgBatchStep, error) {
				calls++
				if calls > 1 {
					<-ctx.Done()
					return svgBatchStep{}, ctx.Err()
				}
				s := batchStep(svgPanelState{8, batchFixture(t, 8)}, svgPanelState{7, svgScriptResult{Status: tc.status}})
				s.Layout.Panels[1].Active = tc.active
				return s, nil
			})
			require.NoError(t, err)
			batch := got.Payload.(BatchCollection)
			require.Equal(t, "ok", batch.Panels[0].Status)
			require.Equal(t, tc.code, batch.Panels[1].Error.Code)
		})
	}
}

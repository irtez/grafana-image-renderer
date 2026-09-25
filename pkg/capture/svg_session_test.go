package capture

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type svgSessionCollector struct {
	limit  int
	called bool
}

func TestSVGSessionNavigationUsesSharedBudgetAndReturnsJSONTimeout(t *testing.T) {
	cfg := captureTestConfig(true)
	cfg.Timeout = 80 * time.Millisecond
	c := &svgSessionCollector{}
	e, err := NewEngine(cfg, c)
	require.NoError(t, err)
	s, err := e.Match(mustTargetURL(t, mapTarget), validTransport())
	require.NoError(t, err)
	// Time spent before navigation is part of the same budget.
	s.startedAt = time.Now().Add(-40 * time.Millisecond)
	require.NoError(t, s.Navigate(t.Context(), func(ctx context.Context) error {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.Less(t, time.Until(deadline), 40*time.Millisecond)
		<-ctx.Done()
		return ctx.Err()
	}))
	body, err := s.Capture(t.Context())
	require.NoError(t, err)
	var got BatchEnvelope
	require.NoError(t, json.Unmarshal(body, &got))
	require.Equal(t, "failed", got.Status)
	require.Equal(t, "CAPTURE_TIMEOUT", got.Error.Code)
	require.Len(t, got.Panels, 2)
	require.False(t, c.called)
}

func TestSVGSessionCancellationIsNotAnEnvelope(t *testing.T) {
	e, err := NewEngine(captureTestConfig(true), &svgSessionCollector{})
	require.NoError(t, err)
	s, err := e.Match(mustTargetURL(t, mapTarget), validTransport())
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = s.Capture(ctx)
	require.ErrorIs(t, err, context.Canceled)
}

func (c *svgSessionCollector) Kind() string { return "svgmodifier" }
func (c *svgSessionCollector) Collect(_ context.Context, r Request, limit int) (Collection, error) {
	c.limit = limit
	c.called = true
	rows := make([]PanelResult, len(r.PanelIDs))
	for i, id := range r.PanelIDs {
		rows[i] = PanelResult{PanelID: id, Status: "ok", Payload: json.RawMessage(`{"kind":"svgmodifier"}`)}
	}
	return Collection{Payload: BatchCollection{Panels: rows}}, nil
}
func TestSVGSessionReturnsV2AndDoesNotUseTableByteLimit(t *testing.T) {
	cfg := captureTestConfig(true)
	cfg.MaxJSONBytes = 1
	cfg.SVGMaxPanelBytes = 10000
	cfg.SVGMaxJSONBytes = 20000
	c := &svgSessionCollector{}
	e, err := NewEngine(cfg, c)
	require.NoError(t, err)
	s, err := e.Match(mustTargetURL(t, mapTarget), validTransport())
	require.NoError(t, err)
	body, err := s.Capture(t.Context())
	require.NoError(t, err)
	var got BatchEnvelope
	require.NoError(t, json.Unmarshal(body, &got))
	require.Equal(t, ContractV2, got.Contract)
	require.Equal(t, "complete", got.Status)
	require.Equal(t, 10000, c.limit)
	require.Equal(t, "now-3h", got.Capture.From)
}

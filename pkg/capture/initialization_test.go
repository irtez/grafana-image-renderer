package capture

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type initializingCollector struct {
	literalCollector
	initialize func(context.Context, Request, int) error
}

func (c initializingCollector) Initialize(ctx context.Context, request Request, maxBytes int) error {
	return c.initialize(ctx, request, maxBytes)
}

func TestSessionInitializationIsAbsentForExistingCollectors(t *testing.T) {
	session := matchedSession(t, captureTestConfig(true), literalCollector{kind: "grafana-table"})
	require.Nil(t, session.Initialization())
}

func TestSessionInitializationReceivesMatchedRequestAndLimit(t *testing.T) {
	var got Request
	var limit, calls int
	cfg := captureTestConfig(true)
	cfg.MaxJSONBytes = 3456
	collector := initializingCollector{
		literalCollector: literalCollector{kind: "grafana-table"},
		initialize: func(ctx context.Context, request Request, maxBytes int) error {
			require.Equal(t, t.Context(), ctx)
			got, limit = request, maxBytes
			calls++
			return nil
		},
	}
	session := matchedSession(t, cfg, collector)
	initialize := session.Initialization()
	require.NotNil(t, initialize)
	require.Zero(t, calls, "подготовка action не должна запускать browser работу")
	require.NoError(t, initialize(t.Context()))
	require.Equal(t, session.Request(), got)
	require.Equal(t, 3456, limit)
	require.Equal(t, 1, calls)
}

func TestSessionInitializationPropagatesFailure(t *testing.T) {
	want := errors.New("bootstrap failed")
	session := matchedSession(t, captureTestConfig(true), initializingCollector{
		literalCollector: literalCollector{kind: "grafana-table"},
		initialize:       func(context.Context, Request, int) error { return want },
	})
	require.ErrorIs(t, session.Initialization()(t.Context()), want)
}

func TestSessionInitializationFallsBackToMaxJSONBytes(t *testing.T) {
	t.Parallel()
	var gotLimit int
	collector := initializingCollector{
		literalCollector: literalCollector{kind: "svgmodifier"},
		initialize: func(ctx context.Context, request Request, maxBytes int) error {
			gotLimit = maxBytes
			return nil
		},
	}
	cfg := captureTestConfig(true)
	engine, err := NewEngine(cfg, collector)
	require.NoError(t, err)
	svgTarget := mustTargetURL(t, "http://grafana:3000/d/cm-sla/dashboard?render=1&siamCapturePanels=4&from=100&to=200&tz=Europe%2FMoscow&siamCaptureVersion=2&siamCaptureKind=svgmodifier")
	session, err := engine.Match(svgTarget, validTransport())
	require.NoError(t, err)
	// Force maxPanelJSONBytes to 0 to trigger fallback
	session.maxPanelJSONBytes = 0
	session.maxJSONBytes = 7777
	require.NotNil(t, session.Initialization())
	err = session.Initialization()(t.Context())
	require.NoError(t, err)
	require.Equal(t, 7777, gotLimit)
}

func TestSessionUsesProducerReadinessWithSVGCollector(t *testing.T) {
	t.Parallel()
	engine, err := NewEngine(captureTestConfig(true), NewSVGCollector())
	require.NoError(t, err)
	svgTarget := mustTargetURL(t, "http://grafana:3000/d/cm-sla/dashboard?render=1&siamCapturePanels=4&from=100&to=200&tz=Europe%2FMoscow&siamCaptureVersion=2&siamCaptureKind=svgmodifier")
	session, err := engine.Match(svgTarget, validTransport())
	require.NoError(t, err)
	require.NotNil(t, session)
	require.True(t, session.UsesProducerReadiness())
}

func TestSessionUsesProducerReadinessWithTableCollector(t *testing.T) {
	t.Parallel()
	engine, err := NewEngine(captureTestConfig(true), NewTableCollector())
	require.NoError(t, err)
	session, err := engine.Match(mustTargetURL(t, validCaptureTarget), validTransport())
	require.NoError(t, err)
	require.NotNil(t, session)
	require.False(t, session.UsesProducerReadiness())
}

func TestSessionInitializationWithSVGCollector(t *testing.T) {
	t.Parallel()
	engine, err := NewEngine(captureTestConfig(true), NewSVGCollector())
	require.NoError(t, err)
	svgTarget := mustTargetURL(t, "http://grafana:3000/d/cm-sla/dashboard?render=1&siamCapturePanels=4&from=100&to=200&tz=Europe%2FMoscow&siamCaptureVersion=2&siamCaptureKind=svgmodifier")
	session, err := engine.Match(svgTarget, validTransport())
	require.NoError(t, err)
	require.NotNil(t, session)
	init := session.Initialization()
	require.NotNil(t, init)
}

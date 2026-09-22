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

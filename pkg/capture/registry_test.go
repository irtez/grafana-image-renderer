package capture

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/grafana/grafana-image-renderer/pkg/config"
	"github.com/stretchr/testify/require"
)

type literalCollector struct {
	kind       string
	collection Collection
	err        error
	wait       bool
}

func (c literalCollector) Kind() string { return c.kind }

func (c literalCollector) Collect(ctx context.Context, _ Request, _ int) (Collection, error) {
	if c.wait {
		<-ctx.Done()
		return Collection{}, ctx.Err()
	}
	if c.collection.Payload == nil && c.collection.Error == nil && c.err == nil {
		return Collection{Payload: literalPayload{Kind: "literal", Value: "ok"}}, nil
	}
	return c.collection, c.err
}

func TestRegistryRejectsEmptyAndDuplicateKinds(t *testing.T) {
	_, err := NewEngine(captureTestConfig(true), literalCollector{})
	require.ErrorContains(t, err, "collector kind must not be empty")

	_, err = NewEngine(captureTestConfig(true),
		literalCollector{kind: "grafana-table"},
		literalCollector{kind: "grafana-table"},
	)
	require.ErrorContains(t, err, "duplicate collector kind")
}

func TestSessionEncodesCollectorDomainError(t *testing.T) {
	collector := literalCollector{
		kind: "grafana-table",
		collection: Collection{Error: &Error{
			Code:    "TABLE_QUERY_ERROR",
			Message: "table query failed",
		}},
	}
	session := matchedSession(t, captureTestConfig(true), collector)

	body, err := session.Capture(t.Context())

	require.NoError(t, err)
	requireCaptureError(t, body, "TABLE_QUERY_ERROR")
}

func TestSessionMapsTimeoutToBoundedError(t *testing.T) {
	cfg := captureTestConfig(true)
	cfg.Timeout = time.Millisecond
	session := matchedSession(t, cfg, literalCollector{kind: "grafana-table", wait: true})

	body, err := session.Capture(t.Context())

	require.NoError(t, err)
	requireCaptureError(t, body, "CAPTURE_TIMEOUT")
}

func TestSessionMapsUnexpectedCollectorErrorWithoutLeakingDetails(t *testing.T) {
	secret := "datasource returned secret-cell-value"
	session := matchedSession(t, captureTestConfig(true), literalCollector{
		kind: "grafana-table",
		err:  errors.New(secret),
	})

	body, err := session.Capture(t.Context())

	require.NoError(t, err)
	requireCaptureError(t, body, "CAPTURE_INTERNAL_ERROR")
	require.NotContains(t, string(body), secret)
}

func TestSessionCapturesSuccessWithFreshMetadata(t *testing.T) {
	session := matchedSession(t, captureTestConfig(true), literalCollector{kind: "grafana-table"})

	body, err := session.Capture(t.Context())

	require.NoError(t, err)
	var envelope struct {
		Capture Metadata `json:"capture"`
		Result  struct {
			Status string `json:"status"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	require.Equal(t, "ok", envelope.Result.Status)
	require.Equal(t, "cm-sla", envelope.Capture.DashboardUID)
	require.Positive(t, envelope.Capture.CapturedAtMs)
}

func matchedSession(t *testing.T, cfg config.CaptureConfig, collector Collector) *Session {
	t.Helper()
	engine, err := NewEngine(cfg, collector)
	require.NoError(t, err)
	session, err := engine.Match(mustTargetURL(t, validCaptureTarget), validTransport())
	require.NoError(t, err)
	require.NotNil(t, session)
	return session
}

func requireCaptureError(t *testing.T, body []byte, code string) {
	t.Helper()
	var envelope struct {
		Result struct {
			Status string `json:"status"`
			Error  Error  `json:"error"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	require.Equal(t, "error", envelope.Result.Status)
	require.Equal(t, code, envelope.Result.Error.Code)
}

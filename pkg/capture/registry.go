package capture

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/grafana/grafana-image-renderer/pkg/config"
)

type Collector interface {
	Kind() string
	Collect(context.Context, Request, int) (Collection, error)
}

type Engine struct {
	config     config.CaptureConfig
	collectors map[string]Collector
}

func NewEngine(cfg config.CaptureConfig, collectors ...Collector) (*Engine, error) {
	if cfg.Timeout <= 0 {
		return nil, fmt.Errorf("capture timeout must be positive")
	}
	if cfg.MaxJSONBytes <= 0 {
		return nil, fmt.Errorf("capture max JSON bytes must be positive")
	}

	registry := make(map[string]Collector, len(collectors))
	for _, collector := range collectors {
		kind := collector.Kind()
		if kind == "" {
			return nil, fmt.Errorf("collector kind must not be empty")
		}
		if _, exists := registry[kind]; exists {
			return nil, fmt.Errorf("duplicate collector kind %q", kind)
		}
		registry[kind] = collector
	}

	return &Engine{config: cfg, collectors: registry}, nil
}

func (e *Engine) Match(target *url.URL, transport Transport) (*Session, error) {
	request, navigationURL, kind, err := parseCaptureRequest(target, transport)
	if err != nil {
		return nil, err
	}
	if kind == "" {
		return nil, nil
	}
	if !e.config.SemanticEnabled {
		return nil, markerInvalid()
	}
	collector, ok := e.collectors[kind]
	if !ok {
		return nil, unsupportedKind()
	}

	return &Session{
		request:       request,
		navigationURL: navigationURL,
		collector:     collector,
		timeout:       e.config.Timeout,
		maxJSONBytes:  e.config.MaxJSONBytes,
	}, nil
}

type Session struct {
	request       Request
	navigationURL string
	collector     Collector
	timeout       time.Duration
	maxJSONBytes  int
}

func (s *Session) Request() Request {
	return s.request
}

func (s *Session) NavigationURL() string {
	return s.navigationURL
}

func (s *Session) Capture(parent context.Context) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, s.timeout)
	defer cancel()

	kind := s.collector.Kind()
	result := "ok"
	collectStarted := time.Now()
	collection, err := s.collector.Collect(ctx, s.request, s.maxJSONBytes)
	MetricSemanticCaptureStageDuration.WithLabelValues(kind, "collect").Observe(time.Since(collectStarted).Seconds())
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded), errors.Is(ctx.Err(), context.DeadlineExceeded):
			result = "timeout"
			collection = Collection{Error: &Error{
				Code:    "CAPTURE_TIMEOUT",
				Message: "semantic capture timed out",
			}}
		default:
			result = "internal_error"
			collection = Collection{Error: &Error{
				Code:    "CAPTURE_INTERNAL_ERROR",
				Message: "semantic capture failed",
			}}
		}
	} else if collection.Error != nil {
		result = "domain_error"
		if collection.Error.Code == "CAPTURE_TIMEOUT" {
			result = "timeout"
		}
	}

	serializeStarted := time.Now()
	body, marshalErr := MarshalCollection(Metadata{
		DashboardUID:  s.request.DashboardUID,
		PanelID:       s.request.PanelID,
		Kind:          s.request.Kind,
		RenderFrom:    s.request.RenderFrom,
		RenderTo:      s.request.RenderTo,
		Timezone:      s.request.Timezone,
		VariablesHash: s.request.VariablesHash,
		CapturedAtMs:  time.Now().UnixMilli(),
	}, collection, s.maxJSONBytes)
	MetricSemanticCaptureStageDuration.WithLabelValues(kind, "serialize").Observe(time.Since(serializeStarted).Seconds())
	if marshalErr != nil {
		MetricSemanticCaptureRequests.WithLabelValues(kind, "serialize_error").Inc()
		return nil, marshalErr
	}
	MetricSemanticCapturePayloadBytes.WithLabelValues(kind).Observe(float64(len(body)))
	MetricSemanticCaptureRequests.WithLabelValues(kind, result).Inc()
	return body, nil
}

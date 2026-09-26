package capture

import (
	"context"
	"errors"
	"time"
)

// The work deadline includes navigation. Keep a small fixed reserve for validation,
// serialization and delivery instead of starting another full timeout per panel.
func (s *Session) workContext(parent context.Context) (context.Context, context.CancelFunc) {
	deadline := s.startedAt.Add(s.timeout)
	if outer, ok := parent.Deadline(); ok && outer.Before(deadline) {
		deadline = outer
	}
	reserve := max(time.Duration(0), min(500*time.Millisecond, deadline.Sub(s.startedAt)/10))
	return context.WithDeadline(parent, deadline.Add(-reserve))
}

func (s *Session) Navigate(parent context.Context, action func(context.Context) error) error {
	if s.request.Version != 2 {
		return action(parent)
	}
	ctx, cancel := s.workContext(parent)
	defer cancel()
	err := action(ctx)
	if parent.Err() != nil {
		return parent.Err()
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		s.navigationError = svgError("CAPTURE_TIMEOUT").Error
		return nil
	}
	return err
}

func (s *Session) captureSVG(parent context.Context) ([]byte, error) {
	if parent.Err() != nil {
		return nil, parent.Err()
	}
	ctx, cancel := s.workContext(parent)
	defer cancel()
	global := s.navigationError
	var panels []PanelResult
	collectStarted := time.Now()
	if global == nil {
		collection, err := s.collector.Collect(ctx, s.request, s.maxPanelJSONBytes)
		if parent.Err() != nil {
			return nil, parent.Err()
		}
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			global = svgError("CAPTURE_TIMEOUT").Error
		case err != nil:
			global = svgError("CAPTURE_INTERNAL_ERROR").Error
		case collection.Error != nil:
			global = collection.Error
		default:
			if batch, ok := collection.Payload.(BatchCollection); ok {
				panels = batch.Panels
			} else {
				global = svgError("CAPTURE_INTERNAL_ERROR").Error
			}
		}
	}
	MetricSemanticCaptureStageDuration.WithLabelValues(s.request.Kind, "collect").Observe(time.Since(collectStarted).Seconds())
	started := time.Now()
	body, status, err := MarshalBatch(s.request, panels, global, s.startedAt.UnixMilli(), started.UnixMilli(), s.maxJSONBytes)
	MetricSemanticCaptureStageDuration.WithLabelValues(s.request.Kind, "serialize").Observe(time.Since(started).Seconds())
	outcome := "ok"
	if err != nil {
		outcome = "serialize_error"
	} else {
		MetricSemanticCapturePayloadBytes.WithLabelValues(s.request.Kind).Observe(float64(len(body)))
		if status != "complete" {
			outcome = "domain_error"
		}
	}
	MetricSemanticCaptureRequests.WithLabelValues(s.request.Kind, outcome).Inc()
	return body, err
}

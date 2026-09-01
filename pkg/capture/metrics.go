package capture

import "github.com/prometheus/client_golang/prometheus"

var (
	MetricSemanticCaptureRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "semantic_capture_requests_total",
		Help: "Number of semantic capture requests by allowlisted kind and bounded result.",
	}, []string{"kind", "result"})

	MetricSemanticCaptureStageDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "semantic_capture_stage_duration_seconds",
		Help:    "Semantic capture stage duration by allowlisted kind.",
		Buckets: prometheus.DefBuckets,
	}, []string{"kind", "stage"})

	MetricSemanticCapturePayloadBytes = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "semantic_capture_payload_bytes",
		Help:    "Final serialized semantic capture response size in bytes.",
		Buckets: []float64{1024, 4 * 1024, 16 * 1024, 64 * 1024, 256 * 1024, 512 * 1024, 1024 * 1024, 2 * 1024 * 1024},
	}, []string{"kind"})
)

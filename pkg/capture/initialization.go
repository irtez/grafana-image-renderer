package capture

import "context"

// BrowserInitializer нужен только коллекторам, которым требуется hook до загрузки страницы.
type BrowserInitializer interface {
	Initialize(context.Context, Request, int) error
}

type ProducerReadiness interface {
	UsesProducerReadiness() bool
}

// Для producer terminal/timeout определяется его протоколом, а не готовностью PNG.
func (s *Session) UsesProducerReadiness() bool {
	readiness, ok := s.collector.(ProducerReadiness)
	return ok && readiness.UsesProducerReadiness()
}

// Initialization не добавляет browser action обычным коллекторам, включая TableNG.
func (s *Session) Initialization() func(context.Context) error {
	initializer, ok := s.collector.(BrowserInitializer)
	if !ok {
		return nil
	}
	return func(ctx context.Context) error {
		limit := s.maxPanelJSONBytes
		if limit == 0 {
			limit = s.maxJSONBytes
		}
		return initializer.Initialize(ctx, s.request, limit)
	}
}

package service

import "github.com/grafana/chromedp"

type preNavigationPrinter interface {
	beforeNavigate() chromedp.Action
}

func readinessActions(printer Printer, imageReadiness func() chromedp.Tasks) chromedp.Tasks {
	if producer, ok := printer.(interface{ usesProducerReadiness() bool }); ok && producer.usesProducerReadiness() {
		return nil
	}
	return imageReadiness()
}

// Навигация остаётся единственным action для PNG/PDF и collectors без bootstrap.
func navigationActions(printer Printer, navigate chromedp.Action) chromedp.Tasks {
	var actions chromedp.Tasks
	if initializer, ok := printer.(preNavigationPrinter); ok {
		if initialize := initializer.beforeNavigate(); initialize != nil {
			actions = append(actions, observingAction("printer.beforeNavigate", initialize))
		}
	}
	return append(actions, observingAction("Navigate", navigate))
}

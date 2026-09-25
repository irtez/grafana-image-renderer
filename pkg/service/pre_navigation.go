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
func navigationActions(printer Printer, navigate chromedp.Action, after ...chromedp.Action) chromedp.Tasks {
	var actions chromedp.Tasks
	if initializer, ok := printer.(preNavigationPrinter); ok {
		if initialize := initializer.beforeNavigate(); initialize != nil {
			actions = append(actions, observingAction("printer.beforeNavigate", initialize))
		}
	}
	work := append(chromedp.Tasks{observingAction("Navigate", navigate)}, after...)
	if budget, ok := printer.(interface {
		navigation(chromedp.Tasks) chromedp.Action
	}); ok {
		return append(actions, budget.navigation(work))
	}
	return append(actions, work...)
}

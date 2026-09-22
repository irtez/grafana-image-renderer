package service

import (
	"context"
	"errors"
	"testing"

	"github.com/grafana/chromedp"
	"github.com/stretchr/testify/require"
)

type initializingPrinter struct {
	Printer
	initialize    chromedp.Action
	producerReady bool
}

func (p initializingPrinter) beforeNavigate() chromedp.Action { return p.initialize }
func (p initializingPrinter) usesProducerReadiness() bool     { return p.producerReady }

func TestProducerReadinessDoesNotWaitForImageBinding(t *testing.T) {
	printer := initializingPrinter{producerReady: true}
	prepared := false
	actions := readinessActions(printer, func() chromedp.Tasks {
		prepared = true
		return chromedp.Tasks{chromedp.ActionFunc(func(context.Context) error { return errors.New("image readiness timeout") })}
	})
	require.False(t, prepared, "не создаём image-readiness listeners для собственного протокола producer")
	require.NoError(t, actions.Do(t.Context()))
}

func TestOrdinaryReadinessKeepsImageWait(t *testing.T) {
	printer, err := NewPNGPrinter()
	require.NoError(t, err)
	waited := false
	actions := readinessActions(printer, func() chromedp.Tasks {
		return chromedp.Tasks{chromedp.ActionFunc(func(context.Context) error { waited = true; return nil })}
	})
	require.NoError(t, actions.Do(t.Context()))
	require.True(t, waited)
}

func TestNavigationInstallsReceiverBeforePageScript(t *testing.T) {
	var events []string
	printer := initializingPrinter{initialize: chromedp.ActionFunc(func(context.Context) error {
		events = append(events, "bootstrap")
		return nil
	})}
	navigate := chromedp.ActionFunc(func(context.Context) error {
		require.Equal(t, []string{"bootstrap"}, events, "inline producer не должен опередить receiver")
		events = append(events, "navigate")
		return nil
	})
	require.NoError(t, navigationActions(printer, navigate).Do(t.Context()))
	require.Equal(t, []string{"bootstrap", "navigate"}, events)
}

func TestNavigationDoesNotContinueAfterBootstrapFailure(t *testing.T) {
	want := errors.New("bootstrap failed")
	printer := initializingPrinter{initialize: chromedp.ActionFunc(func(context.Context) error { return want })}
	navigated := false
	navigate := chromedp.ActionFunc(func(context.Context) error { navigated = true; return nil })
	require.ErrorIs(t, navigationActions(printer, navigate).Do(t.Context()), want)
	require.False(t, navigated)
}

func TestOrdinaryNavigationDoesNotAddInitializationAction(t *testing.T) {
	png, err := NewPNGPrinter()
	require.NoError(t, err)
	pdf, err := NewPDFPrinter()
	require.NoError(t, err)
	for _, printer := range []Printer{png, pdf, initializingPrinter{}} {
		calls := 0
		tasks := navigationActions(printer, chromedp.ActionFunc(func(context.Context) error {
			calls++
			return nil
		}))
		require.Len(t, tasks, 1, "обычный путь содержит только прежнюю navigation")
		require.NoError(t, tasks.Do(t.Context()))
		require.Equal(t, 1, calls)
	}
}

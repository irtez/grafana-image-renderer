package service

import (
	"context"

	"github.com/grafana/chromedp"
	"github.com/grafana/grafana-image-renderer/pkg/capture"
	"github.com/grafana/grafana-image-renderer/pkg/config"
)

type capturePrinter struct {
	session *capture.Session
}

func NewCapturePrinter(session *capture.Session) Printer {
	return &capturePrinter{session: session}
}

func (p *capturePrinter) prepare(_ config.BrowserConfig, _ string) chromedp.Action {
	return chromedp.ActionFunc(func(context.Context) error {
		return nil
	})
}

func (p *capturePrinter) action(output chan []byte, _ config.BrowserConfig, _ string) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		body, err := p.session.Capture(ctx)
		if err != nil {
			return err
		}
		output <- body
		return nil
	})
}

func (p *capturePrinter) contentType() string {
	return "application/json"
}

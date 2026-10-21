package service

import (
	"testing"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/grafana/grafana-image-renderer/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldTrackReadinessNetworkRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		event    *network.EventRequestWillBeSent
		expected bool
	}{
		{
			name:     "nil event",
			event:    nil,
			expected: false,
		},
		{
			name:     "nil request payload",
			event:    &network.EventRequestWillBeSent{},
			expected: false,
		},
		{
			name: "http URL",
			event: &network.EventRequestWillBeSent{
				Request: &network.Request{URL: "http://example.com/path"},
			},
			expected: true,
		},
		{
			name: "https URL uppercase scheme",
			event: &network.EventRequestWillBeSent{
				Request: &network.Request{URL: "HTTPS://example.com/path"},
			},
			expected: true,
		},
		{
			name: "blob URL",
			event: &network.EventRequestWillBeSent{
				Request: &network.Request{URL: "blob:http://example.com/worker"},
			},
			expected: false,
		},
		{
			name: "event source request",
			event: &network.EventRequestWillBeSent{
				Type:    network.ResourceTypeEventSource,
				Request: &network.Request{URL: "https://example.com/events"},
			},
			expected: false,
		},
		{
			name: "websocket request",
			event: &network.EventRequestWillBeSent{
				Type:    network.ResourceTypeWebSocket,
				Request: &network.Request{URL: "wss://example.com/live"},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, shouldTrackReadinessNetworkRequest(tt.event))
		})
	}
}

func TestReadinessNetworkObserverTracksUniqueRequestIDs(t *testing.T) {
	t.Parallel()

	observer := newReadinessNetworkObserver()

	first := &network.EventRequestWillBeSent{
		RequestID: network.RequestID("req-1"),
		Request:   &network.Request{URL: "https://example.com/main.js"},
	}
	assert.True(t, observer.onRequestWillBeSent(first))
	assert.Equal(t, int64(1), observer.inflight())

	// Redirect / duplicate request events for the same RequestID should not increase inflight count.
	assert.False(t, observer.onRequestWillBeSent(first))
	assert.Equal(t, int64(1), observer.inflight())

	observer.onRequestCompleted(network.RequestID("req-1"))
	assert.Equal(t, int64(0), observer.inflight())

	// Duplicate completion events should be ignored and never make inflight negative.
	observer.onRequestCompleted(network.RequestID("req-1"))
	assert.Equal(t, int64(0), observer.inflight())
}

func TestReadinessNetworkObserverIgnoresUntrackedRequests(t *testing.T) {
	t.Parallel()

	observer := newReadinessNetworkObserver()

	assert.False(t, observer.onRequestWillBeSent(&network.EventRequestWillBeSent{
		RequestID: network.RequestID("blob-1"),
		Request:   &network.Request{URL: "blob:https://example.com/worker"},
	}))
	assert.Equal(t, int64(0), observer.inflight())

	observer.onRequestCompleted(network.RequestID("blob-1"))
	assert.Equal(t, int64(0), observer.inflight())
}

func TestPaperSize_UnmarshalText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		wantErr bool
		want    PaperSize
	}{
		{"A4", "a4", false, PaperA4},
		{"A4 uppercase", "A4", false, PaperA4},
		{"Letter", "letter", false, PaperLetter},
		{"Legal", "legal", false, PaperLegal},
		{"Tabloid", "tabloid", false, PaperTabloid},
		{"Ledger", "ledger", false, PaperLedger},
		{"A0", "a0", false, PaperA0},
		{"A1", "a1", false, PaperA1},
		{"A2", "a2", false, PaperA2},
		{"A3", "a3", false, PaperA3},
		{"A5", "a5", false, PaperA5},
		{"A6", "a6", false, PaperA6},
		{"Invalid", "invalid", true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var ps PaperSize
			err := ps.UnmarshalText([]byte(tt.input))
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, ps)
			}
		})
	}
}

func TestPaperSize_UnmarshalJSON(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		wantErr bool
		want    PaperSize
	}{
		{"A4", `"a4"`, false, PaperA4},
		{"Letter", `"letter"`, false, PaperLetter},
		{"Invalid", `"invalid"`, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var ps PaperSize
			err := ps.UnmarshalJSON([]byte(tt.input))
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, ps)
			}
		})
	}
}

func TestPaperSize_FormatInches(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		ps     PaperSize
		width  float64
		height float64
	}{
		{"Letter", PaperLetter, 8.5, 11},
		{"Legal", PaperLegal, 8.5, 14},
		{"Tabloid", PaperTabloid, 11, 17},
		{"Ledger", PaperLedger, 17, 11},
		{"A0", PaperA0, 33.1102, 46.80},
		{"A1", PaperA1, 23.39, 33.1102},
		{"A2", PaperA2, 16.5354, 23.39},
		{"A3", PaperA3, 11.70, 16.5354},
		{"A4", PaperA4, 8.2677, 11.70},
		{"A5", PaperA5, 5.8268, 8.2677},
		{"A6", PaperA6, 4.1339, 5.8268},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			width, height, err := tt.ps.FormatInches()
			require.NoError(t, err)
			require.Equal(t, tt.width, width)
			require.Equal(t, tt.height, height)
		})
	}
}

func TestPaperSize_FormatInches_Invalid(t *testing.T) {
	t.Parallel()
	var ps PaperSize
	width, height, err := ps.FormatInches()
	require.Error(t, err)
	require.Equal(t, -1.0, width)
	require.Equal(t, -1.0, height)
}

func TestNewPNGPrinter(t *testing.T) {
	t.Parallel()
	printer, err := NewPNGPrinter()
	require.NoError(t, err)
	require.NotNil(t, printer)
}

func TestNewPNGPrinter_WithFullHeight(t *testing.T) {
	t.Parallel()
	printer, err := NewPNGPrinter(WithFullHeight(true))
	require.NoError(t, err)
	require.NotNil(t, printer)
}

func TestNewPDFPrinter(t *testing.T) {
	t.Parallel()
	printer, err := NewPDFPrinter()
	require.NoError(t, err)
	require.NotNil(t, printer)
}

func TestNewPDFPrinter_WithPaperSize(t *testing.T) {
	t.Parallel()
	printer, err := NewPDFPrinter(WithPaperSize(PaperA4))
	require.NoError(t, err)
	require.NotNil(t, printer)
}

func TestNewPDFPrinter_WithPrintingBackground(t *testing.T) {
	t.Parallel()
	printer, err := NewPDFPrinter(WithPrintingBackground(true))
	require.NoError(t, err)
	require.NotNil(t, printer)
}

func TestNewPDFPrinter_WithPageRanges(t *testing.T) {
	t.Parallel()
	printer, err := NewPDFPrinter(WithPageRanges("1-3"))
	require.NoError(t, err)
	require.NotNil(t, printer)
}

func TestNewPDFPrinter_AllOptions(t *testing.T) {
	t.Parallel()
	printer, err := NewPDFPrinter(
		WithPaperSize(PaperA3),
		WithPrintingBackground(false),
		WithPageRanges("*"),
	)
	require.NoError(t, err)
	require.NotNil(t, printer)
}

func TestWithPageScaleFactor(t *testing.T) {
	t.Parallel()
	opt := WithPageScaleFactor(2.0)
	cfg := config.BrowserConfig{}
	newCfg, err := opt(cfg)
	require.NoError(t, err)
	require.NotNil(t, newCfg.ApplyAll)
}

func TestWithPageScaleFactor_Zero(t *testing.T) {
	t.Parallel()
	opt := WithPageScaleFactor(0)
	cfg := config.BrowserConfig{}
	_, err := opt(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "positive")
}

func TestWithPageScaleFactor_Negative(t *testing.T) {
	t.Parallel()
	opt := WithPageScaleFactor(-1.0)
	cfg := config.BrowserConfig{}
	_, err := opt(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "positive")
}

func TestWithLandscape_True(t *testing.T) {
	t.Parallel()
	opt := WithLandscape(true)
	cfg := config.BrowserConfig{}
	newCfg, err := opt(cfg)
	require.NoError(t, err)
	require.NotNil(t, newCfg.ApplyAll)
}

func TestWithLandscape_False(t *testing.T) {
	t.Parallel()
	opt := WithLandscape(false)
	cfg := config.BrowserConfig{}
	newCfg, err := opt(cfg)
	require.NoError(t, err)
	require.NotNil(t, newCfg.ApplyAll)
}

func TestWithHeader(t *testing.T) {
	t.Parallel()
	opt := WithHeader("X-Custom-Header", "test-value")
	cfg := config.BrowserConfig{}
	newCfg, err := opt(cfg)
	require.NoError(t, err)
	require.NotNil(t, newCfg.Headers)
}

func TestWithHeader_EmptyName(t *testing.T) {
	t.Parallel()
	opt := WithHeader("", "value")
	cfg := config.BrowserConfig{}
	_, err := opt(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "header name was empty")
}

func TestWithCookie(t *testing.T) {
	t.Parallel()
	opt := WithCookie("session", "abc123", "example.com")
	cfg := config.BrowserConfig{}
	newCfg, err := opt(cfg)
	require.NoError(t, err)
	require.Len(t, newCfg.Cookies, 1)
	require.Equal(t, "session", newCfg.Cookies[0].Name)
	require.Equal(t, "abc123", newCfg.Cookies[0].Value)
	require.Equal(t, "example.com", newCfg.Cookies[0].Domain)
}

func TestWithCookie_IPv6(t *testing.T) {
	t.Parallel()
	opt := WithCookie("token", "xyz", "2001:db8::1")
	cfg := config.BrowserConfig{}
	newCfg, err := opt(cfg)
	require.NoError(t, err)
	require.Len(t, newCfg.Cookies, 1)
	require.Equal(t, "[2001:db8::1]", newCfg.Cookies[0].Domain)
}

func TestWithTimeZone(t *testing.T) {
	t.Parallel()
	loc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	opt := WithTimeZone(loc)
	cfg := config.BrowserConfig{}
	newCfg, err := opt(cfg)
	require.NoError(t, err)
	require.Equal(t, loc, newCfg.TimeZone)
}

func TestWithTimeZone_Nil(t *testing.T) {
	t.Parallel()
	opt := WithTimeZone(nil)
	cfg := config.BrowserConfig{}
	_, err := opt(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil")
}

func TestWithTimeZone_Empty(t *testing.T) {
	t.Parallel()
	opt := WithTimeZone(&time.Location{})
	cfg := config.BrowserConfig{}
	_, err := opt(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty")
}

func TestWithViewport(t *testing.T) {
	t.Parallel()
	opt := WithViewport(1920, 1080)
	cfg := config.BrowserConfig{}
	newCfg, err := opt(cfg)
	require.NoError(t, err)
	require.NotNil(t, newCfg.ApplyAll)
}

func TestWithViewport_IgnoreNegative(t *testing.T) {
	t.Parallel()
	opt := WithViewport(-1, -1)
	cfg := config.BrowserConfig{}
	newCfg, err := opt(cfg)
	require.NoError(t, err)
	require.NotNil(t, newCfg.ApplyAll)
}

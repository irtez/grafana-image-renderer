package capture

import (
	"net/url"
	"testing"
	"time"

	"github.com/grafana/grafana-image-renderer/pkg/config"
	"github.com/stretchr/testify/require"
)

const validCaptureTarget = "http://grafana:3000/render/d-solo/cm-sla/dashboard?render=1&panelId=4&from=100&to=200&tz=Europe%2FMoscow&var-b=y&var-a=1&var-b=x&siamCaptureVersion=1&siamCaptureKind=grafana-table"

func captureTestConfig(enabled bool) config.CaptureConfig {
	return config.CaptureConfig{
		SemanticEnabled: enabled,
		Timeout:         time.Second,
		MaxJSONBytes:    4096,
	}
}

func mustTargetURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	target, err := url.Parse(raw)
	require.NoError(t, err)
	return target
}

func requireProtocolCode(t *testing.T, err error, code string) {
	t.Helper()
	var protocolErr *ProtocolError
	require.ErrorAs(t, err, &protocolErr)
	require.Equal(t, code, protocolErr.Code)
}

func TestMarkerParsesIdentityAndCleansNavigationURL(t *testing.T) {
	engine, err := NewEngine(captureTestConfig(true), literalCollector{kind: "grafana-table"})
	require.NoError(t, err)
	target := mustTargetURL(t, validCaptureTarget)
	original := target.String()

	session, err := engine.Match(target, Transport{Encoding: "png", RenderKey: "rk", Domain: "grafana"})

	require.NoError(t, err)
	require.NotNil(t, session)
	require.Equal(t, Request{
		DashboardUID:  "cm-sla",
		PanelID:       4,
		Kind:          "grafana-table",
		RenderFrom:    "100",
		RenderTo:      "200",
		Timezone:      "Europe/Moscow",
		VariablesHash: "sha256:62d0d0e798e64da03c58233b241bea53d200d90577bc9587abd9d717ace7cca4",
	}, session.Request())
	require.Equal(t, original, target.String(), "Match must not mutate the caller URL")

	cleaned := mustTargetURL(t, session.NavigationURL())
	require.Empty(t, cleaned.Query()["siamCaptureVersion"])
	require.Empty(t, cleaned.Query()["siamCaptureKind"])
	require.Equal(t, []string{"y", "x"}, cleaned.Query()["var-b"])
	require.Equal(t, "1", cleaned.Query().Get("render"))
}

func TestMarkerAbsentKeepsNormalRenderPath(t *testing.T) {
	engine, err := NewEngine(captureTestConfig(false))
	require.NoError(t, err)

	session, err := engine.Match(
		mustTargetURL(t, "http://grafana:3000/render/d-solo/cm-sla/dashboard?render=1&panelId=4"),
		Transport{Encoding: "pdf"},
	)

	require.NoError(t, err)
	require.Nil(t, session)
}

func TestMarkerRejectsInvalidContracts(t *testing.T) {
	tests := []struct {
		name      string
		mutateURL func(*url.URL)
		transport Transport
		enabled   bool
	}{
		{name: "only version", enabled: true, transport: validTransport(), mutateURL: deleteQuery("siamCaptureKind")},
		{name: "only kind", enabled: true, transport: validTransport(), mutateURL: deleteQuery("siamCaptureVersion")},
		{name: "duplicate version", enabled: true, transport: validTransport(), mutateURL: appendRaw("&siamCaptureVersion=1")},
		{name: "version two", enabled: true, transport: validTransport(), mutateURL: setQuery("siamCaptureVersion", "2")},
		{name: "unknown marker", enabled: true, transport: validTransport(), mutateURL: setQuery("siamCaptureFoo", "x")},
		{name: "disabled", enabled: false, transport: validTransport()},
		{name: "non render path", enabled: true, transport: validTransport(), mutateURL: setPath("/d-solo/cm-sla/dashboard")},
		{name: "dashboard render", enabled: true, transport: validTransport(), mutateURL: setPath("/render/d/cm-sla/dashboard")},
		{name: "empty uid", enabled: true, transport: validTransport(), mutateURL: setPath("/render/d-solo//dashboard")},
		{name: "empty slug", enabled: true, transport: validTransport(), mutateURL: setPath("/render/d-solo/cm-sla/")},
		{name: "dot uid", enabled: true, transport: validTransport(), mutateURL: setPath("/render/d-solo/./dashboard")},
		{name: "dotdot slug", enabled: true, transport: validTransport(), mutateURL: setPath("/render/d-solo/cm-sla/..")},
		{name: "missing panel", enabled: true, transport: validTransport(), mutateURL: deleteQuery("panelId")},
		{name: "duplicate panel", enabled: true, transport: validTransport(), mutateURL: appendRaw("&panelId=4")},
		{name: "noncanonical panel", enabled: true, transport: validTransport(), mutateURL: setQuery("panelId", "04")},
		{name: "negative panel", enabled: true, transport: validTransport(), mutateURL: setQuery("panelId", "-1")},
		{name: "missing from", enabled: true, transport: validTransport(), mutateURL: deleteQuery("from")},
		{name: "duplicate to", enabled: true, transport: validTransport(), mutateURL: appendRaw("&to=200")},
		{name: "empty timezone", enabled: true, transport: validTransport(), mutateURL: setQuery("tz", "")},
		{name: "missing render", enabled: true, transport: validTransport(), mutateURL: deleteQuery("render")},
		{name: "render zero", enabled: true, transport: validTransport(), mutateURL: setQuery("render", "0")},
		{name: "pdf", enabled: true, transport: Transport{Encoding: "pdf", RenderKey: "rk", Domain: "grafana"}},
		{name: "empty render key", enabled: true, transport: Transport{Encoding: "png", Domain: "grafana"}},
		{name: "empty domain", enabled: true, transport: Transport{Encoding: "png", RenderKey: "rk"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine, err := NewEngine(captureTestConfig(tt.enabled), literalCollector{kind: "grafana-table"})
			require.NoError(t, err)
			target := mustTargetURL(t, validCaptureTarget)
			if tt.mutateURL != nil {
				tt.mutateURL(target)
			}

			session, err := engine.Match(target, tt.transport)

			require.Nil(t, session)
			requireProtocolCode(t, err, "CAPTURE_MARKER_INVALID")
		})
	}
}

func TestMarkerRejectsUnregisteredKind(t *testing.T) {
	engine, err := NewEngine(captureTestConfig(true))
	require.NoError(t, err)

	session, err := engine.Match(mustTargetURL(t, validCaptureTarget), validTransport())

	require.Nil(t, session)
	requireProtocolCode(t, err, "CAPTURE_KIND_UNSUPPORTED")
}

func TestVariablesHashCanonicalizesRepeatedUnicodeValues(t *testing.T) {
	values := url.Values{
		"ignored": {"x"},
		"var-z":   {"<&>", "line\u2028separator"},
		"var-a":   {"ёж", ""},
	}

	hash := VariablesHash(values)

	require.Equal(t, "sha256:1ccb52a5a13492ab447e2721542453791911f29db7f966b99c2b80c3394d6841", hash)
	require.Equal(t, "sha256:4f53cda18c2baa0c0354bb5f9a3ecbe5ed12ab4d8e11ba873c2f11161202b945", VariablesHash(url.Values{}))
}

func validTransport() Transport {
	return Transport{Encoding: "png", RenderKey: "rk", Domain: "grafana"}
}

func deleteQuery(key string) func(*url.URL) {
	return func(target *url.URL) {
		query := target.Query()
		query.Del(key)
		target.RawQuery = query.Encode()
	}
}

func setQuery(key, value string) func(*url.URL) {
	return func(target *url.URL) {
		query := target.Query()
		query.Set(key, value)
		target.RawQuery = query.Encode()
	}
}

func appendRaw(value string) func(*url.URL) {
	return func(target *url.URL) {
		target.RawQuery += value
	}
}

func setPath(value string) func(*url.URL) {
	return func(target *url.URL) {
		target.Path = value
		target.RawPath = ""
	}
}

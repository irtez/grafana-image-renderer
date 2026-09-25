package capture

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/grafana/chromedp"
	"github.com/stretchr/testify/require"
)

// Opt-in real Chromium exercise of the embedded registry, scene adapter and Go
// validator together. Fixtures are authored here and contain no dashboard export.
func TestSVGBrowserCapture(t *testing.T) {
	path := os.Getenv("SVG_CAPTURE_BROWSER")
	if path == "" {
		t.Skip("set SVG_CAPTURE_BROWSER to run real Chromium contract checks")
	}
	fixture, err := os.ReadFile("testdata/svg-snapshot-v2.json")
	require.NoError(t, err)
	for _, tc := range []struct{ name, script, status string }{
		{"delayed sibling", `setTimeout(()=>publish(8),150)`, "complete"},
		{"pending sibling", ``, "partial"},
		{"unmounted sibling", `setTimeout(()=>{handles[7].close();panels[0].isActive=false},100);setTimeout(()=>publish(8),180)`, "complete"},
		{"new generation", `setTimeout(()=>handles[7].begin({...run,generation:2}),100);setTimeout(()=>publish(8),180)`, "partial"},
		{"changed context", `setTimeout(()=>scene.state.$timeRange.state.value.to=2000,100)`, "failed"},
		{"variable loading changes context", `setTimeout(()=>scene.state.$variables.state.variables=[{state:{name:'zone',value:'changed',loading:true}}],100)`, "failed"},
		{"initial variable loading", `scene.state.defaultVariablesLoading=true;setTimeout(()=>{scene.state.defaultVariablesLoading=false;publish(8)},150)`, "complete"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			html := fmt.Sprintf(`<html><body><div data-viz-panel-key="panel-7"></div><div data-viz-panel-key="panel-8"></div><script>
const snapshot=%s,run={generation:1,effectiveFromMs:0,effectiveToMs:1000};
const scene={state:{uid:'example',version:1,$timeRange:{state:{from:'now-3h',to:'now',timeZone:'UTC',value:{from:0,to:1000}}},$variables:{state:{variables:[]}}},getDashboardPanels(){return panels}};
const panels=[7,8].map(id=>({state:{key:'panel-'+id,pluginId:'svgmodifier-panel'},parent:scene,isActive:true,getLegacyPanelId(){return id}}));
window.__grafanaSceneContext=scene;const handles={};
for(const id of [7,8]){handles[id]=__SVG_MODIFIER_CAPTURE_V2__.connect({producerId:'svgmodifier-panel',producerVersion:'1.4.0',panelId:id,instanceId:'test-'+id});handles[id].begin(run)}
function publish(id){const value=JSON.parse(JSON.stringify(snapshot));value.panel.id=id;handles[id].publish(1,()=>value)}
publish(7);%s;
</script></body></html>`, fixture, tc.script)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				_, _ = w.Write([]byte(html))
			}))
			defer server.Close()
			alloc, cancel := chromedp.NewExecAllocator(t.Context(), append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(path), chromedp.NoSandbox)...)
			defer cancel()
			browser, closeBrowser := chromedp.NewContext(alloc)
			defer closeBrowser()
			ctx, cancelDeadline := context.WithTimeout(browser, 10*time.Second)
			defer cancelDeadline()
			require.NoError(t, chromedp.Run(ctx))
			cfg := captureTestConfig(true)
			cfg.Timeout = 800 * time.Millisecond
			engine, err := NewEngine(cfg, NewSVGCollector())
			require.NoError(t, err)
			target, err := url.Parse(server.URL + "/d/example/map?render=1&from=now-3h&to=now&tz=UTC&siamCaptureKind=svgmodifier&siamCaptureVersion=2&siamCapturePanels=7,8")
			require.NoError(t, err)
			session, err := engine.Match(target, validTransport())
			require.NoError(t, err)
			var body []byte
			require.NoError(t, chromedp.Run(ctx, chromedp.ActionFunc(session.Initialization()), chromedp.ActionFunc(func(ctx context.Context) error {
				return session.Navigate(ctx, chromedp.Tasks{chromedp.Navigate(session.NavigationURL()), chromedp.WaitReady("body")}.Do)
			}), chromedp.ActionFunc(func(ctx context.Context) error { var err error; body, err = session.Capture(ctx); return err })))
			var envelope BatchEnvelope
			require.NoError(t, json.Unmarshal(body, &envelope))
			require.Equal(t, tc.status, envelope.Status, string(body))
			require.Len(t, envelope.Panels, 2)
			if tc.name == "new generation" {
				require.Equal(t, "CAPTURE_TIMEOUT", envelope.Panels[0].Error.Code)
				require.Equal(t, "ok", envelope.Panels[1].Status)
			}
			if tc.name == "changed context" || tc.name == "variable loading changes context" {
				require.Equal(t, "CAPTURE_CONTEXT_CHANGED", envelope.Error.Code)
			}
		})
	}
}

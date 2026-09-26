# Siam semantic panel capture

This fork adds an opt-in semantic response to the existing Grafana image-rendering flow. Grafana still authenticates the caller, creates the normal `renderKey`, and calls the renderer's existing `/render` route. No public renderer route or second authentication scheme is added.

Two collectors share the route, with independently versioned contracts:

- `grafana-table`: displayed text from the rendered TableNG ARIA grid after normal image readiness. It does not read raw DataFrames or Inspector CSV.
- `svgmodifier` v2: compact snapshots published by a compatible SVG Modifier plugin, for one panel or an explicit list on one dashboard. The plugin supplies object names/bindings, indicators, metrics, tooltip contents and navigation links. The renderer validates and transports these facts; it does not calculate thresholds, name colors, group services or format metric values.

## Configuration

Semantic capture is disabled by default.

| CLI flag | Environment variable | Default |
|---|---|---|
| `--capture.semantic-enabled` | `CAPTURE_SEMANTIC_ENABLED` | `false` |
| `--capture.timeout` | `CAPTURE_TIMEOUT` | `5s` |
| `--capture.max-json-bytes` | `CAPTURE_MAX_JSON_BYTES` | `1048576` |
| `--capture.svg-max-panels` | `CAPTURE_SVG_MAX_PANELS` | `16` |
| `--capture.svg-max-panel-bytes` | `CAPTURE_SVG_MAX_PANEL_BYTES` | `4194304` |
| `--capture.svg-max-json-bytes` | `CAPTURE_SVG_MAX_JSON_BYTES` | `16777216` |

Equivalent YAML:

```yaml
capture:
  semantic-enabled: true
  timeout: 5s
  max-json-bytes: 1048576
  svg-max-panels: 16
  svg-max-panel-bytes: 4194304
  svg-max-json-bytes: 16777216
```

All limits must be positive. `max-json-bytes` applies only to TableNG. SVG has separate panel/count/total limits; the total includes envelope metadata and errors.

## Request path

Clients continue to call Grafana:

```text
GET /render/d-solo/{dashboardUid}/{slug}
  ?panelId=4
  &from=1788112800000
  &to=1788116400000
  &tz=Europe/Moscow
  &siamCaptureVersion=1
  &siamCaptureKind=grafana-table
```

Grafana proxies this through its normal remote-renderer integration. For TableNG v1 the renderer accepts semantic mode only when all of these conditions hold:

- both marker parameters occur exactly once;
- no other `siamCapture*` parameter is present;
- the Chromium navigation path produced by Grafana is `/d-solo/{uid}/{slug}` with canonical `panelId`, non-empty `from`, `to`, and `tz`, and `render=1`;
- the outer renderer request uses `encoding=png` and has the existing non-empty `renderKey` and `domain`;
- the feature is enabled and the collector kind is registered.

The marker is removed before Chromium navigation. An absent marker follows the unchanged PNG/PDF path. A partial or invalid marker fails closed and never falls back to a screenshot.

For SVG Modifier use version **2**. Solo accepts `panelId=4` or `panelId=panel-4`. Dashboard mode requires a non-empty ordered list of distinct numeric IDs, without `panelId`:

```text
GET /render/d-solo/{dashboardUid}/{slug}?panelId=panel-4&from=now-3h&to=now&tz=Europe/Moscow&siamCaptureVersion=2&siamCaptureKind=svgmodifier
GET /render/d/{dashboardUid}/{slug}?from=now-3h&to=now&tz=Europe/Moscow&siamCaptureVersion=2&siamCaptureKind=svgmodifier&siamCapturePanels=8,4
```

The outer renderer call still requires `encoding=png`, `renderKey` and `domain`. Subpaths are supported. Capture markers and auto-refresh URL parameters are removed before navigation; time and variable selections are preserved. There is no SVG v1 fallback, automatic panel discovery, `include*` switch or new endpoint.

## Response

Inside the renderer the semantic response is `application/json`. Grafana 12.3.8 and 13.1.3 forward these bytes while retaining the outer `image/png` content type in the tested remote-renderer flow. Semantic clients must validate the strict JSON body instead of trusting that outer MIME type.

Example TableNG v1 success envelope (unchanged):

```json
{"contract":"siam-render-capture/v1","capture":{"dashboardUid":"cm-sla","panelId":4,"kind":"grafana-table","renderFrom":"1788112800000","renderTo":"1788116400000","timezone":"Europe/Moscow","variablesHash":"sha256:...","capturedAtMs":1788116401000},"result":{"status":"ok","payload":{"kind":"grafana-table","schemaVersion":1,"panel":{"id":4,"title":"up по системам"},"observed":{"dataState":"Done","effectiveFromMs":1788112800000,"effectiveToMs":1788116400000},"frame":{"index":0,"name":"A","totalFrames":1},"columns":["Time","Value"],"dataset":{"totalRows":1,"capturedRows":1,"complete":true,"limitedBy":null},"rows":[{"position":0,"cells":["2026-08-31 21:00:00","1"]}]}}}
```

For `grafana-table`, rows and cells are never sliced. The limit is applied to final serialized UTF-8 bytes; complete rows are removed from the end until the envelope fits. `dataset.limitedBy="json_bytes"` identifies a valid prefix. If one row cannot fit an otherwise empty response, the renderer returns `TABLE_ROW_TOO_LARGE` without its cell values.

### SVG v2 batch response

Both solo and dashboard mode return `contract: "siam-render-capture/v2"` with:

- `capture`: `dashboardUid`, ordered `requestedPanelIds`, original `from`/`to`/`timezone`, `variablesHash`, `startedAtMs`, `capturedAtMs`;
- `status`: `complete` (all snapshots), `partial` (some), or `failed` (none);
- `panels`: one entry per requested ID, in request order. Success is `{panelId,status:"ok",payload}`; failure is `{panelId,status:"error",error:{code,message}}`;
- optional top-level `error` for a global failure. It discards all payloads and marks every requested panel with an error. Invalid request markers return `capture:null` and `panels:[]`, without reflecting unvalidated inputs.

Each `panels[].payload` conforms to [the embedded schema](../pkg/capture/svgmodifier-snapshot-v2.schema.json). A [complete payload example](../pkg/capture/testdata/svg-snapshot-v2.json) includes `objects`, `indicators`, `rules`, `metrics`, `expressions`, `links` and `diagnostics`. It contains no full SVG tree or source time-series points. Actual paint includes numeric RGBA; color naming/filtering is a consumer concern.

The renderer checks schema, producer/panel/run identity, indices and references, then embeds the raw JSON. Numbers, strings, nulls and array order are preserved; whitespace and JSON escaping are not a byte-for-byte contract. Requested time in `capture` may differ from observed time in the payload because panel overrides are allowed. The renderer does not replace observed time with requested time.

Each panel's SVG output is **complete or an error**. Neither table rows inside a payload nor other arrays are trimmed. Per-panel overflow affects that panel; final envelope overflow returns `CAPTURE_PAYLOAD_TOO_LARGE` for the whole batch, never an arbitrary prefix. Configuration and missing-data diagnostics can be part of an `ok` snapshot: envelope status describes collection, not service health. A configured plugin grid/table display mode returns `CAPTURE_MODE_UNSUPPORTED`; SVG-drawn tables and table metrics within SVG remain supported.

### Producer lifecycle and limits

Only SVG v2 requests install `__SVG_MODIFIER_CAPTURE_V2__` before navigation. Its per-panel registry accepts selected panel IDs only. The plugin connects with its identity, begins a generation and publishes one snapshot or a safe failure. A new generation invalidates the previous result. Late callbacks, closed instances and duplicate publications cannot revive an old snapshot. Multiple live matching instances are an error, not an arbitrary selection.

SVG v2 uses one `CAPTURE_TIMEOUT` budget from request matching, including browser startup/navigation and all panels. The earlier outer deadline wins; up to 500 ms (10% for short budgets) is reserved for the reply. SVG does not wait for image readiness. Table capture and ordinary rendering retain their existing readiness path and timeout behavior. Normal requests install no SVG hook, frame scan or polling loop; schema compilation is lazy on the first SVG capture.

The Grafana 13.1.3 scene adapter inventories exact panel IDs, disables refresh, opens selected rows/tabs and scrolls lazy panels in this temporary browser page only. No dashboard is saved. Completed snapshots survive tab unmount, but a newer producer generation invalidates an old snapshot. Changed dashboard/time/variables abort the batch rather than combine incompatible results. Repeated panels are not supported. Unknown scene/layout APIs fail explicitly. At deadline, ready panels remain usable and unfinished panels receive errors.

The default five seconds is not a latency guarantee for a dashboard. Size it for the target data sources, with room inside the outer render request timeout. Client cancellation/browser infrastructure failures can still abort transport before JSON delivery.

The receiver retains an immutable bounded plain-JSON copy before CDP transfer. Accessors, functions, cycles, non-finite values and unsupported objects are rejected. Limits include UTF-8 bytes, depth 64, a traversal budget derived from the byte limit, 64 live connections per panel and a frame scan bounded to 64 contexts/depth 16. Go rejects duplicate JSON keys and bounds numeric tokens before schema validation. Unchanged snapshots are not transferred repeatedly while waiting for another panel.

These are data/copy limits, not a JavaScript execution sandbox. A producer factory, Proxy trap, or the engine's initial own-key enumeration can run before a copy budget stops the traversal. Capture timeout bounds the wait for a response; it cannot preempt arbitrary synchronous JavaScript inside the page.

Same-origin frame discovery supports the Grafana frontend sandbox without disabling it. Frames explicitly owned by another panel are skipped; inaccessible/unaccounted-for selected or unscoped contexts fail with `CAPTURE_FRAME_UNSUPPORTED`. An older plugin without v2 returns `CAPTURE_PRODUCER_MISSING` after the wait. Active sandbox compatibility must be rechecked when upgrading Grafana.

## Authentication and exposure

The marker is a mode selector, not a credential. This feature deliberately adds no HMAC, Kubernetes Secret, nonce, key rotation, or direct MCP-to-renderer credentials. Access remains protected by:

- Grafana authentication and dashboard/data-source permissions;
- Grafana's short-lived `renderKey` identity;
- the existing renderer `X-Auth-Token`;
- production ingress and network policy.

## Error codes

Transport and registry errors:

- `CAPTURE_MARKER_INVALID`
- `CAPTURE_KIND_UNSUPPORTED`
- `CAPTURE_TIMEOUT`
- `CAPTURE_PAYLOAD_TOO_LARGE`
- `CAPTURE_INTERNAL_ERROR`

SVG producer and validation errors:

- `CAPTURE_PRODUCER_MISSING`
- `CAPTURE_PROTOCOL_UNSUPPORTED`
- `CAPTURE_INSTANCE_AMBIGUOUS`
- `CAPTURE_FRAME_UNSUPPORTED`
- `CAPTURE_DATA_STATE_UNSUPPORTED`
- `CAPTURE_EXPORT_FAILED`
- `CAPTURE_PAYLOAD_INVALID`
- `CAPTURE_VALIDATION_LIMIT`
- `CAPTURE_PANEL_NOT_FOUND`
- `CAPTURE_PANEL_UNSUPPORTED`
- `CAPTURE_MODE_UNSUPPORTED`
- `CAPTURE_REPEAT_UNSUPPORTED`
- `CAPTURE_LAYOUT_UNSUPPORTED`
- `CAPTURE_CONTEXT_CHANGED`
- `CAPTURE_SVG_COMPLEXITY_LIMIT`
- `CAPTURE_SVG_TEXT_LIMIT`
- `CAPTURE_SVG_INVALID_GEOMETRY`

Table errors:

- `TABLE_PANEL_NOT_FOUND`
- `TABLE_PANEL_TYPE_MISMATCH`
- `TABLE_DATA_NOT_READY`
- `TABLE_QUERY_ERROR`
- `TABLE_FRAME_MISSING`
- `TABLE_NESTED_FRAME_UNSUPPORTED`
- `TABLE_GRID_NOT_FOUND`
- `TABLE_GRID_INCONSISTENT`
- `TABLE_PAGINATION_STALLED`
- `TABLE_ROW_TOO_LARGE`

Messages are bounded and do not contain URLs, credentials, variables, query responses, or cell values.

## Metrics

- `semantic_capture_requests_total{kind,result}` where result is `ok`, `domain_error`, `timeout`, `internal_error`, or `serialize_error`;
- `semantic_capture_stage_duration_seconds{kind,stage}` where stage is `collect` or `serialize`;
- `semantic_capture_payload_bytes{kind}`.

For SVG v2, the request outcome follows the final serialized envelope: `complete`
counts as `ok`, while `partial` and `failed` count as `domain_error`. This includes
a total-byte overflow that replaces successfully collected panels with a failed
envelope, as well as envelopes containing panel timeout errors. If serialization
fails or even the error envelope cannot fit, the outcome is `serialize_error`.
TableNG retains its existing timeout and internal-error outcomes.

Labels contain only allowlisted collector kinds and fixed enums. Dashboard IDs, panel IDs, URLs, variables, values, and error messages are not labels.

## Compatibility and rollout

The TableNG collector depends on ARIA roles/indexes and the literal `.table-ng-pagination` hook, originally checked on Grafana `12.3.8`; ingress has also been checked on `13.1.3`. SVG integration targets Grafana `13.1.3`. The fork baseline is Grafana Image Renderer `5.8.11`. A version upgrade requires re-running semantic ingress and the upstream renderer acceptance suite, including the active frontend sandbox when enabled.

The [maintained test catalogue](semantic-test-cases.md) describes expected behavior and the corresponding automated tests. A new or changed behavior test must update that catalogue in the same change.

Rollout sequence:

Upgrade the SVG plugin and renderer together: v1 SVG requests/producers are not supported. TableNG remains on v1. Consumers must migrate to the ordered v2 envelope and use the three SVG-specific limits above.

1. deploy with `CAPTURE_SEMANTIC_ENABLED=false`;
2. enable it in a restricted environment;
3. run the Grafana-ingress JSON smoke plus ordinary PNG/PDF regressions;
4. enable the MCP consumer only after that gate passes.

Rollback is `CAPTURE_SEMANTIC_ENABLED=false` followed by the normal renderer restart. Ordinary render paths do not depend on semantic capture.

Alert list collection, Inspector CSV as a semantic source, a separate endpoint, PNG/iTXt metadata, and renderer-side pagination are intentionally not implemented. Ordinary CSV export remains independent of semantic capture.

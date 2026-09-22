# Siam semantic panel capture

This fork adds an opt-in semantic response to the existing Grafana image-rendering flow. Grafana still authenticates the caller, creates the normal `renderKey`, and calls the renderer's existing `/render` route. No public renderer route or second authentication scheme is added.

Two collectors use the same envelope and route:

- `grafana-table`: displayed text from the rendered TableNG ARIA grid after normal image readiness. It does not read raw DataFrames or Inspector CSV.
- `svgmodifier`: the complete version-1 snapshot published by a compatible SVG Modifier plugin. The renderer validates the snapshot; it does not calculate thresholds, choose colors, group services, or format metric values.

## Configuration

Semantic capture is disabled by default.

| CLI flag | Environment variable | Default |
|---|---|---|
| `--capture.semantic-enabled` | `CAPTURE_SEMANTIC_ENABLED` | `false` |
| `--capture.timeout` | `CAPTURE_TIMEOUT` | `5s` |
| `--capture.max-json-bytes` | `CAPTURE_MAX_JSON_BYTES` | `1048576` |

Equivalent YAML:

```yaml
capture:
  semantic-enabled: true
  timeout: 5s
  max-json-bytes: 1048576
```

Timeout and byte limit must be positive; invalid values stop server startup.

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

Grafana proxies this through its normal remote-renderer integration. The renderer accepts semantic mode only when all of these conditions hold:

- both marker parameters occur exactly once;
- no other `siamCapture*` parameter is present;
- the Chromium navigation path produced by Grafana is `/d-solo/{uid}/{slug}` with canonical `panelId`, non-empty `from`, `to`, and `tz`, and `render=1`;
- the outer renderer request uses `encoding=png` and has the existing non-empty `renderKey` and `domain`;
- the feature is enabled and the collector kind is registered.

The marker is removed before Chromium navigation. An absent marker follows the unchanged PNG/PDF path. A partial or invalid marker fails closed and never falls back to a screenshot.

For SVG Modifier, use `siamCaptureKind=svgmodifier`; the other request parameters and authentication stay the same. No `include*` switches or additional endpoint are needed.

## Response

Inside the renderer the semantic response is `application/json`. Grafana 12.3.8 and 13.1.3 forward these bytes while retaining the outer `image/png` content type in the tested remote-renderer flow. Semantic clients must validate the strict JSON body instead of trusting that outer MIME type.

Example success envelope:

```json
{"contract":"siam-render-capture/v1","capture":{"dashboardUid":"cm-sla","panelId":4,"kind":"grafana-table","renderFrom":"1788112800000","renderTo":"1788116400000","timezone":"Europe/Moscow","variablesHash":"sha256:...","capturedAtMs":1788116401000},"result":{"status":"ok","payload":{"kind":"grafana-table","schemaVersion":1,"panel":{"id":4,"title":"up по системам"},"observed":{"dataState":"Done","effectiveFromMs":1788112800000,"effectiveToMs":1788116400000},"frame":{"index":0,"name":"A","totalFrames":1},"columns":["Time","Value"],"dataset":{"totalRows":1,"capturedRows":1,"complete":true,"limitedBy":null},"rows":[{"position":0,"cells":["2026-08-31 21:00:00","1"]}]}}}
```

For `grafana-table`, rows and cells are never sliced. The limit is applied to final serialized UTF-8 bytes; complete rows are removed from the end until the envelope fits. `dataset.limitedBy="json_bytes"` identifies a valid prefix. If one row cannot fit an otherwise empty response, the renderer returns `TABLE_ROW_TOO_LARGE` without its cell values.

### SVG snapshot

`result.payload` conforms to [the embedded schema](../pkg/capture/svgmodifier-snapshot-v1.schema.json). A [complete example](../pkg/capture/testdata/svg-snapshot-v1.json) contains panel and observed-time metadata, expressions, configured elements, scalar and table metrics, diagnostics, and diagram facts such as authored IDs, text, paint and bounds. These are producer facts, not inferred relationships or a human-readable summary. Historical source points are not included.

The renderer checks schema, producer/panel/run identity, indices and references, then embeds the raw JSON. Numbers, strings, nulls and array order are preserved; whitespace and JSON escaping are not a byte-for-byte contract. Requested time in `capture` may differ from observed time in the payload because panel overrides are allowed. The renderer does not replace observed time with requested time.

SVG output is **complete or an error**. Neither table rows inside this payload nor other arrays are trimmed. A snapshot that fits on its own but exceeds the final envelope limit also returns `CAPTURE_PAYLOAD_TOO_LARGE`. Configuration and missing-data diagnostics can be part of a successful snapshot; a transport error means no usable snapshot was delivered.

### Producer lifecycle and limits

Only SVG semantic requests install `__SVG_MODIFIER_CAPTURE_V1__` before navigation. The plugin connects with its identity, begins a generation and publishes one snapshot or a safe failure. A new generation invalidates the previous result. Late callbacks, closed instances and duplicate publications cannot revive an old snapshot. Multiple live matching instances are an error, not an arbitrary selection.

After the page body is available, SVG capture waits on this lifecycle under `CAPTURE_TIMEOUT`. It does not wait for the image-rendering completion binding: a producer must be able to report an unsupported data state or failure even if image readiness never completes. Table capture and ordinary rendering retain their existing readiness path. Normal requests install no SVG hook, frame scan or polling loop; schema compilation is lazy on the first SVG capture.

For SVG, this timeout includes waiting for the initial data and export code to finish loading. The default five seconds is not a latency guarantee for a dashboard. Size it for the target data sources, with room inside the outer render request timeout; an outer timeout may end the request before a semantic error can be returned.

The receiver accepts plain JSON data and retains an immutable bounded copy before CDP transfer. Accessor properties, functions, cycles, non-finite numbers and unsupported object types are rejected. Limits include UTF-8 bytes, depth 64, 100,000 visited values, 64 live connections, and a frame scan bounded to 64 contexts and depth 16. The Go validator also bounds numeric tokens and validates the full envelope size.

These are data/copy limits, not a JavaScript execution sandbox. A producer factory, Proxy trap, or the engine's initial own-key enumeration can run before a copy budget stops the traversal. Capture timeout bounds the wait for a response; it cannot preempt arbitrary synchronous JavaScript inside the page.

Same-origin frame discovery supports the Grafana frontend sandbox without disabling it or adding plugin-visible access helpers. Inaccessible documents or an unaccounted-for child context fail with `CAPTURE_FRAME_UNSUPPORTED`; no partial frame result is returned. An older plugin without the export protocol returns `CAPTURE_PRODUCER_MISSING` after the capture wait.

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

Labels contain only allowlisted collector kinds and fixed enums. Dashboard IDs, panel IDs, URLs, variables, values, and error messages are not labels.

## Compatibility and rollout

The TableNG collector depends on ARIA roles/indexes and the literal `.table-ng-pagination` hook, originally checked on Grafana `12.3.8`; ingress has also been checked on `13.1.3`. SVG integration targets Grafana `13.1.3`. The fork baseline is Grafana Image Renderer `5.8.11`. A version upgrade requires re-running semantic ingress and the upstream renderer acceptance suite, including the active frontend sandbox when enabled.

The [maintained test catalogue](semantic-test-cases.md) describes expected behavior and the corresponding automated tests. A new or changed behavior test must update that catalogue in the same change.

Rollout sequence:

1. deploy with `CAPTURE_SEMANTIC_ENABLED=false`;
2. enable it in a restricted environment;
3. run the Grafana-ingress JSON smoke plus ordinary PNG/PDF regressions;
4. enable the MCP consumer only after that gate passes.

Rollback is `CAPTURE_SEMANTIC_ENABLED=false` followed by the normal renderer restart. Ordinary render paths do not depend on semantic capture.

Alert list collection, Inspector CSV as a semantic source, a separate endpoint, PNG/iTXt metadata, and renderer-side pagination are intentionally not implemented. Ordinary CSV export remains independent of semantic capture.

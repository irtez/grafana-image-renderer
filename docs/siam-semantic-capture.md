# Siam semantic panel capture

This fork adds an opt-in semantic response to the existing Grafana image-rendering flow. Grafana still authenticates the caller, creates the normal `renderKey`, and calls the renderer's existing `/render` route. No public renderer route or second authentication scheme is added.

The only supported collector in the first version is `grafana-table/v1`. It reads text from the rendered Grafana 12.3.8 TableNG ARIA grid after normal renderer readiness. It does not use raw DataFrame display processors, Inspector CSV, screenshots, or PNG metadata.

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

## Response

Inside the renderer the semantic response is `application/json`. Grafana 12.3.8 currently forwards the bytes while retaining its outer `image/png` content type, so semantic clients must validate the strict JSON body instead of trusting that outer MIME type.

Example success envelope:

```json
{"contract":"siam-render-capture/v1","capture":{"dashboardUid":"cm-sla","panelId":4,"kind":"grafana-table","renderFrom":"1788112800000","renderTo":"1788116400000","timezone":"Europe/Moscow","variablesHash":"sha256:...","capturedAtMs":1788116401000},"result":{"status":"ok","payload":{"kind":"grafana-table","schemaVersion":1,"panel":{"id":4,"title":"up по системам"},"observed":{"dataState":"Done","effectiveFromMs":1788112800000,"effectiveToMs":1788116400000},"frame":{"index":0,"name":"A","totalFrames":1},"columns":["Time","Value"],"dataset":{"totalRows":1,"capturedRows":1,"complete":true,"limitedBy":null},"rows":[{"position":0,"cells":["2026-08-31 21:00:00","1"]}]}}}
```

Rows and cells are never sliced. The limit is applied to final serialized UTF-8 bytes; complete rows are removed from the end until the envelope fits. `dataset.limitedBy="json_bytes"` identifies a valid prefix. If one row cannot fit an otherwise empty response, the renderer returns `TABLE_ROW_TOO_LARGE` without its cell values.

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

The DOM collector is pinned to Grafana `12.3.8` TableNG roles/indexes and the literal `.table-ng-pagination` hook. The fork baseline is Grafana Image Renderer `5.8.11`. A version upgrade requires re-running the semantic ingress smoke and the upstream renderer acceptance suite.

Rollout sequence:

1. deploy with `CAPTURE_SEMANTIC_ENABLED=false`;
2. enable it in a restricted environment;
3. run the Grafana-ingress JSON smoke plus ordinary PNG/PDF regressions;
4. enable the MCP consumer only after that gate passes.

Rollback is `CAPTURE_SEMANTIC_ENABLED=false` followed by the normal renderer restart. Ordinary render paths do not depend on semantic capture.

Alert list collection, SVG Modifier, Inspector CSV, a separate endpoint, PNG/iTXt metadata, and renderer-side pagination are intentionally not implemented in this slice.

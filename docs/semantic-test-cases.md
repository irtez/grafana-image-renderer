# Semantic capture: behavior and tests

This catalogue describes the current contract, not a migration checklist. When adding or changing a behavior test, update the corresponding row here in the same change. Related parameterized cases can share a row; name the edge cases explicitly. Fixtures must remain synthetic and self-contained.

Run the automated Go suites with:

```sh
go test ./pkg/capture ./pkg/service ./pkg/api ./pkg/config ./pkg/metrics -count=1
go test ./... -short -count=1
go test -race ./pkg/capture ./pkg/service ./pkg/api
```

Node.js is required for `TestSVGScriptRuntime`: it executes the embedded receiver and reader in isolated JavaScript contexts. Without Node, that test explicitly skips; a green Go result alone then does not verify the JavaScript scenarios. `-short` does not run the upstream Docker acceptance suite.

Run the real Chromium lifecycle/transport check separately:

```sh
SVG_CAPTURE_BROWSER=/path/to/chromium go test ./pkg/capture -run TestSVGBrowserCapture -count=1
```

It covers delayed/pending sibling panels, unmount retention, generation invalidation and changed context through the actual embedded scripts and Go validator. It does not replace a Grafana-ingress check.

## Routing and lifecycle

### SVG capture v2

| Case | Expected behavior | Automated coverage |
|---|---|---|
| V2 solo and explicit dashboard panel list, including subpath | Normalize solo panel-N, preserve requested order and repeated variables; remove capture markers from navigation | `TestV2MarkersSelectOrderedBatchOrSolo` |
| Duplicate/empty/unsafe IDs, mixed selectors, excess panels, v1 SVG | Reject before browser work; no PNG fallback | `TestV2MarkersRejectAmbiguousSelection` |
| V2 invalid request | JSON failed envelope without reflecting unvalidated metadata or credentials | `TestInvalidV2SelectionReturnsJSONWithoutStartingBrowser` |
| Ordered panel results and UTF-8 boundary | complete/partial/failed describes collection; raw numeric precision remains unchanged | `TestV2EnvelopePreservesOrderStatusAndPrecision` |
| Oversized batch | Top-level overflow, every requested ID retained as an error, no arbitrary payload prefix | `TestV2EnvelopeNeverTruncatesAnOversizedBatch` |
| Missing/duplicate/conflicting result entries | Reject malformed internal envelope instead of publishing success | `TestV2EnvelopeRejectsMissingDuplicateOrConflictingEntries` |
| SVG defaults and environment limits | Separate panel/count/total limits do not change TableNG byte default | `pkg/config/capture_test.go` |
| Compact producer v2 and panel overrides | Validate actual identity/run without substituting the requested time window; return original numeric tokens | `TestValidateV2SnapshotPreservesProducerPayload`, `svg_test.go` |
| Object/rule/metric/diagnostic links, winners, cyclic parents/causes | Reject unknown, conflicting or dangling references without panic | `TestValidateV2SnapshotRejectsSchemaReferencesAndStates` |
| Table source rows and displayed order | Validate row widths, winner/cell issue indices and tooltip references independently | `TestValidateV2TableKeepsSourceIndicesAndTooltipOrder` |
| JSON preflight | Reject duplicate keys, trailing JSON, invalid UTF-8, excessive depth/numeric tokens before schema allocation; safety limit has a distinct code | `TestValidateV2SnapshotRejectsMalformedJSONAndBoundedWork`, `TestValidateV2SafetyLimitHasDistinctError` |
| Large valid compact payload | No obsolete 100k-values ceiling below the configured byte budget | `TestValidateV2AcceptsLargeJSONWithinByteBudget` |
| V2 browser registry | Two selected panel IDs have independent generations/overflow/errors; duplicate instances affect only their panel; panelId getters do not execute | `TestSVGScriptRuntime` |
| Frame ownership | An inaccessible frame scoped to another panel does not invalidate the selected panel; unscoped/unknown frames still fail closed | `TestSVGScriptRuntime` |

| Case | Expected behavior | Automated coverage |
|---|---|---|
| No capture marker | Ordinary rendering, no SVG bootstrap action or frame polling; image readiness remains enabled. | `TestMarkerAbsentKeepsNormalRenderPath`, `TestOrdinaryNavigationDoesNotAddInitializationAction`, `TestOrdinaryReadinessKeepsImageWait` |
| Invalid, incomplete or duplicate marker; disabled feature; unsupported kind | Reject the request, never silently return PNG instead of JSON. | `marker_test.go`, semantic route tests in `pkg/api` |
| Collector with optional bootstrap | Receive the matched request and byte limit before navigation. Bootstrap failure prevents navigation. Existing collectors need not implement it. | `initialization_test.go`, `pre_navigation_test.go` |
| Producer-owned readiness | SVG may report a result or failure without Grafana's image completion signal. Other collectors retain the old wait. | `TestProducerReadinessDoesNotWaitForImageBinding` |
| Idle / pending at deadline | Idle means `CAPTURE_PRODUCER_MISSING`; pending or not-yet-observed state means `CAPTURE_TIMEOUT`, including a deadline during CDP read. Client cancellation is not a missing producer. | `TestSVGBatchDeadlineDistinguishesMissingPendingAndInactive`, `TestSVGBatchDoesNotConvertClientCancellationToSuccess` |
| Transport error text and metrics | Unknown errors become bounded safe messages. Producer-reported timeout increments the `timeout` outcome, not `domain_error`. | `registry_test.go`, `TestSVGCollectorAllowsOnlySafeErrorCodes` |

## Dashboard collection

| Case | Expected behavior | Automated coverage |
|---|---|---|
| Rows, tabs and lazy panels | Activate exact selected ancestors, scroll panel ID, disable refresh only in the temporary page; no title matching or dashboard save. | `TestSVGLayoutDriver` |
| Missing, wrong type, repeat or unknown layout | Explicit per-panel error; never select the first repeated instance. | `TestSVGLayoutDriver` |
| Completed panel unmounts | Retain its request-local snapshot and requested order while collecting another tab. | `TestSVGBatchRetainsSnapshotBeforeUnmountAndKeepsRequestedOrder` |
| Ready sibling and pending panel | Deadline preserves ready result, pending gets timeout; no full timeout per panel. | `TestSVGBatchDeadlinePreservesReadySibling` |
| New generation after success | Invalidate old payload before pending/timeout, never return stale success. | `TestSVGBatchNewRunInvalidatesRetainedSnapshot` |
| Changed variables/time/dashboard, including loading that never completes | Fail entire inconsistent batch; no cross-context payloads, including deadline during a new variable load. Initial loading remains waitable. | `TestSVGBatchContextChangeRejectsMixedResults`, `TestSVGLayoutDriver`, `TestSVGBrowserCapture/variable_loading_changes_context` |
| Shared deadline and cancellation | Navigation consumes the same budget, with reply reserve. Client cancellation propagates as cancellation. | `svg_session_test.go` |
| Separate v2 limits/output | TableNG byte limit does not affect SVG; solo and batch use the same ordered envelope. | `TestSVGSessionReturnsV2AndDoesNotUseTableByteLimit` |
| Batch HTTP route | One browser call; preserve repeated variables and relative time; remove markers/refresh. | `TestV2BatchUsesCapturePrinterAndPreservesNavigationInputs` |
| Cache stamp transfer | Go emits the exact keys used by the browser to omit unchanged payloads. | `TestSVGBatchCacheStampMatchesBrowserKeys` |
| Initial data loading before plugin mount | Report timeout, not missing producer. A new loading state invalidates retained success even before producer begin. | `TestSVGBatchLoadingWithoutProducerIsTimeoutNotMissingPlugin`, `TestSVGLayoutDriver` |

## Browser receiver and frame reader

The following rows run through `TestSVGScriptRuntime`; individual scenarios live in [svg_receiver_test.cjs](../pkg/capture/testdata/svg_receiver_test.cjs).

| Case | Expected behavior |
|---|---|
| Bootstrap in a child context; unrelated producer/panel | Install without a top-only guard or timers; accept only the requested panel and supported protocol. Repeated bootstrap preserves an active connection. |
| Publish a snapshot | Make one immutable JSON copy, preserve values and count serialized UTF-8 bytes exactly. Later producer mutation cannot change the stored result. |
| New run after success | Immediately remove the old snapshot. Late publish/fail cannot replace the active generation. |
| Malformed new run | Remove old success and expose an invalid-payload error. Do not execute time-property getters. A valid old/duplicate begin does not invalidate the current run. |
| Duplicate or recursive publish | Do not call a factory after success/failure or twice recursively for the same run. A new run started inside an older factory can still publish. |
| Reentrant begin/fail/close during validation or copying | Preserve the newer state; an outer callback cannot overwrite it or revive a closed instance. |
| Unmount/remount and simultaneous instances | Ignore closed handles. Two live connections are ambiguous even with identical identity fields; the live-handle limit cannot hide an extra producer. |
| Factory exception or unknown failure code | Return a fixed safe error, without exception text. |
| Accessors, functions, cycles, unsupported objects, non-finite values | Reject without running getters or `toJSON`. Inherited `toJSON` added after publication cannot alter the stored copy; data keys cannot pollute prototypes. |
| Byte, string, depth, array and work limits | Accept the exact valid boundary; otherwise return a size error with no partial snapshot. Unicode accounting is in bytes, not string length. Wide/non-enumerable properties are included in rejection checks. |
| Nested/hidden same-origin frames and virtualized sandbox document | Discover actual child documents using parent-side native DOM access. Read state only after discovery so a producer update during traversal cannot leak old success. |
| Aliases for one producer | Deduplicate consistent aliases, including equal JSON objects with different key insertion order. Disagreement about generation or payload fails closed. |
| Distinct producer instances across frames | Return ambiguity, even if one instance has already failed. |
| Inaccessible or undiscovered native child; unsupported receiver | Return an explicit safe error even when the top-level producer succeeded. A virtual readable document cannot substitute for an inaccessible native one. |
| Frame count/depth and cyclic aliases | Stop at the bounded limit; do not loop or silently return a partial result. |

## Payload validation and output

| Case | Expected behavior | Automated coverage |
|---|---|---|
| Compact payload, schema and observed panel time | Validate v2 schema, identity and references; preserve producer values, including different panel overrides. | `svg_validation_test.go` |
| No-data/configuration diagnostics | Preserve producer facts; collection success does not imply a healthy service. | `TestValidateV2SnapshotPreservesProducerPayload` |
| Copy/state and byte counters | Terminal snapshot must match identity, run and exact UTF-8 count; no SVG array truncation. | `svg_test.go`, `batch_contract_test.go` |
| Concurrent validators | Share compiled schema without mutating payload or request identity. | `TestValidateV2SnapshotConcurrentCalls` |
| TableNG byte limit | Keep the existing whole-row prefix contract; an oversized first row produces `TABLE_ROW_TOO_LARGE`. SVG tables do not use this truncation policy. | `table_test.go`, `contract_test.go` |

## Browser / HTTP release checks

These checks complement unit tests. Run them on the target Grafana and renderer Chromium versions; record versions, fixed input range, status, body type and timing with the result. Do not label them passed just because the Go suite passed.

| Scenario | Required observation |
|---|---|
| Authenticated Grafana ingress | SVG, table metric, invalid YAML and missing input return validated snapshots; plugin grid mode returns an explicit unsupported error; verify body independently of the outer MIME type. |
| Active frontend sandbox | Prove that the producer runs in the sandbox, then repeat SVG/table capture. Merely setting the sandbox configuration is insufficient. |
| Controlled producer and raw transport | Compare the complete returned payload against the published snapshot, including Unicode and observed time different from the requested range. |
| Failure before image readiness; pending; absent producer | Return the declared safe error, timeout, or missing-producer error instead of an image-readiness timeout or PNG fallback. |
| Oversized snapshot | Explicit overflow and no partial payload. |
| Authorization | Invalid renderer token and unauthenticated Grafana ingress cannot read the snapshot. Markers do not confer access. |
| Ordinary PNG, PDF, CSV and TableNG JSON | All remain usable. With fixed data, compare PNG before/after for timeseries, stat, table and SVG panels. |
| Ordinary render performance | Compare the same sequential and modestly parallel PNG workload before/after on the same runtime, without builds or other browser tests running concurrently. Report median/p95, errors and the resource measurement method. |

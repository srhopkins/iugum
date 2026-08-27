# Observe UI HTTP calls

The Perses UI talks to this same origin (`directUrl` is `""`).
`/` loads `homelab-dashboard.json` (copied into `dist`).
If that file is missing, the UI stays on the empty dashboard.
Temp panels query PromQL such as `junction_c{gpu="mi50"}`.
The log panel queries LogQL `{stream="homelab"}`.
Those panels use the calls below.

Source: `@perses-dev/prometheus-plugin` 0.58.0 (`lib/model/prometheus-client.js`) and `@perses-dev/loki-plugin` 0.6.0 (`lib/model/loki-client.js`).
This UI's `src/` makes no `fetch` of its own. Datasources are in-memory (`src/datasources.js`).

`iugum observe` still serves `GET /`, `GET /meta.json`, `POST /ingest/*`, and `GET /query/*`.
This UI does not call those last three.

## Static — already served (`go:embed` of `dist/`)

| Method | Path | Implement |
| --- | --- | --- |
| GET | `/` | already (`index.html`) |
| GET | `/assets/*` | already (JS, CSS, fonts) |

## Prometheus — implement (`iugum-h7p.6`)

`directUrl` = `""`. POST bodies are `application/x-www-form-urlencoded`. GET params are query-string.
Times are Unix seconds. Optional datasource `queryParams` are extra query-string keys.

Success envelope (all `/api/v1/*` except `/-/healthy`):

```
{ "status": "success", "data": <below>, "warnings": ["..."]?, "infos": ["..."]? }
```

Error envelope:

```
{ "status": "error", "errorType": "...", "error": "...", "data": ...? }
```

`ValueTuple` is `[unixSeconds, "sampleString"]`.
`Metric` is `{ [label]: "value" }`.

| Method | Path | Query / body | Response `data` | When |
| --- | --- | --- | --- | --- |
| GET | `/-/healthy` | none | HTTP 200 only (body ignored) | client health check (nothing in this UI calls it yet) |
| POST | `/api/v1/query` | `query`, optional `time`, `timeout` | `{ resultType: "vector"\|"matrix"\|"scalar"\|"string", result }` | instant PromQL, variables, editor |
| POST | `/api/v1/query_range` | `query`, `start`, `end`, `step`, optional `timeout` | `{ resultType: "matrix", result: [{ metric, values: ValueTuple[] }] }` | range chart, annotations |
| POST | `/api/v1/labels` | optional `start`, `end`, `match[]`, `limit` | `string[]` | label-name variable, explorer |
| GET | `/api/v1/label/{labelName}/values` | optional `start`, `end`, `match[]`, `limit` | `string[]` | label-value variable, explorer |
| GET | `/api/v1/metadata` | optional `limit`, `metric` | `{ [metric]: [{ type, help, unit? }] }` | metrics explorer |
| POST | `/api/v1/series` | `match[]`, optional `start`, `end`, `limit` | `Metric[]` | metrics explorer |
| POST | `/api/v1/parse_query` | `query` | PromQL AST object | PromQL editor tree view |

Sample range body:

```
query=junction_c{gpu="mi50"}&start=1756000000&end=1756003600&step=15
```

Sample instant body:

```
query=junction_c&time=1756003600
```

Vector `result` item: `{ metric, value: ValueTuple, histogram? }`.
Matrix `result` item: `{ metric, values: ValueTuple[], histograms? }`.

## Loki — implement (`iugum-h7p.7`)

`directUrl` = `""`. All calls are GET. Params are query-string.
The plugin also sends `Content-Type: application/json` on those GETs (no body).
`start` / `end` are Unix seconds (`toUnixSeconds` in the client).

Stream row: `{ stream: { [label]: "value" }, values: [["nsTimestamp", "line"], ...] }`.
Vector row: `{ metric, value: [unixSeconds, "sample"] }`.
Matrix row: `{ metric, values: [[unixSeconds, "sample"], ...] }`.

| Method | Path | Query | Response | When |
| --- | --- | --- | --- | --- |
| GET | `/loki/api/v1/query` | `query`, optional `time`, `direction`, `limit` | `{ status, data: { resultType: "vector"\|"streams", result, stats? } }` | instant LogQL (metric query) |
| GET | `/loki/api/v1/query_range` | `query`, `start`, `end`, optional `step`, `interval`, `direction`, `limit` | `{ status, data: { resultType: "matrix"\|"streams", result, stats? } }` | log panel, metric range, LogQL variable |
| GET | `/loki/api/v1/labels` | optional `start`, `end`, `query` | `{ status, data: string[] }` | label-name variable, editor complete |
| GET | `/loki/api/v1/label/{labelName}/values` | optional `start`, `end`, `query` | `{ status, data: string[] }` | label-value variable, editor complete |
| GET | `/loki/api/v1/series` | `match[]`, optional `start`, `end` | `{ status, data: [{ [label]: "value" }] }` | on LokiClient; no caller in 0.6.0 |
| GET | `/loki/api/v1/index/volume` | `query`, `start`, `end`, optional `step`, `limit` | object (plugin types it as `Record`) | on LokiClient; no caller in 0.6.0 |
| GET | `/loki/api/v1/index/volume_range` | `query`, `start`, `end`, optional `step`, `limit` | object (plugin types it as `Record`) | on LokiClient; no caller in 0.6.0 |
| GET | `/loki/api/v1/index/stats` | `query`, optional `start`, `end` | `{ streams, chunks, entries, bytes }` (no `status` wrapper) | on LokiClient; no caller in 0.6.0 |

`direction` is `forward` or `backward`.

Sample range:

```
GET /loki/api/v1/query_range?query={stream="homelab"}&start=1756000000&end=1756003600&limit=200&direction=backward
```

## Ignore — not used by this UI

No Perses server. Dashboard JSON is a stub in `src/datasources.js`.
`PluginRegistry` uses `dynamicImportPluginLoader`, not `remotePluginLoader`.

| Method | Path | Why ignore |
| --- | --- | --- |
| * | `/api/v1/projects/*` | Perses app CRUD; this UI never calls it |
| * | `/api/v1/dashboards/*` | same |
| GET | `/api/v1/plugins` | remote plugin list; this UI bundles plugins |
| GET | `/plugins/*` | remote plugin assets |
| GET | `/loki/api/v1/tail` | Loki editor allow-list only; client has no `tail()` |

These stay on the Go server for agents and ingest:

- `POST /ingest/metrics`
- `POST /ingest/logs`
- `GET /query/metrics?q=&start=&end=&max=`
- `GET /query/logs?q=&limit=`
- `GET /meta.json`

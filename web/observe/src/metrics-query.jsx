import React, { useState } from "react";
import { Box, Button, Stack, TextField, Typography } from "@mui/material";

function metricName(metric) {
  if (!metric) {
    return "";
  }
  const name = metric.__name__ || "";
  const parts = Object.keys(metric)
    .filter((k) => k !== "__name__")
    .sort()
    .map((k) => `${k}="${metric[k]}"`);
  if (parts.length === 0) {
    return name || "(series)";
  }
  return `${name}{${parts.join(",")}}`;
}

export function MetricsQuery() {
  const [expr, setExpr] = useState('junction_c{gpu="mi50"}');
  const [rows, setRows] = useState([]);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  function run(e) {
    if (e) {
      e.preventDefault();
    }
    const end = Math.floor(Date.now() / 1000);
    const start = end - 3600;
    const body = new URLSearchParams({
      query: expr,
      start: String(start),
      end: String(end),
      step: "30",
    });
    setBusy(true);
    fetch("/api/v1/query_range", {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body,
    })
      .then(async (res) => {
        const data = await res.json().catch(() => ({}));
        if (!res.ok || data.status === "error") {
          throw new Error((data.error && data.error) || data.errorType || res.statusText);
        }
        return (data.data && data.data.result) || [];
      })
      .then((result) => {
        setRows(result);
        setErr("");
      })
      .catch((e2) => {
        setErr(e2.message || "query failed");
        setRows([]);
      })
      .finally(() => {
        setBusy(false);
      });
  }

  return (
    <Box>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
        PromQL subset, last 1 hour. Example: {`junction_c{gpu="mi50"}`} or{" "}
        {`rate(junction_c[1m])`}
      </Typography>
      <Stack
        component="form"
        direction="row"
        spacing={1}
        sx={{ mb: 2 }}
        onSubmit={run}
      >
        <TextField
          size="small"
          fullWidth
          label="PromQL"
          value={expr}
          onChange={(ev) => setExpr(ev.target.value)}
        />
        <Button type="submit" variant="contained" disabled={busy}>
          Run
        </Button>
      </Stack>
      {err ? (
        <Typography color="error" sx={{ mb: 1 }}>
          {err}
        </Typography>
      ) : null}
      <Box
        className="dashboard-json"
        sx={{ minHeight: "16rem", overflow: "auto", p: 1 }}
      >
        {rows.length === 0 && !err ? (
          <Typography variant="body2" color="text.secondary">
            No series. Run a query.
          </Typography>
        ) : (
          rows.map((row, i) => {
            const vals = row.values || [];
            const last = vals.length ? vals[vals.length - 1] : null;
            return (
              <Box key={i} sx={{ mb: 0.75 }}>
                <Typography variant="body2">{metricName(row.metric)}</Typography>
                <Typography variant="caption" color="text.secondary">
                  {vals.length} points
                  {last ? ` · last ${last[1]}` : ""}
                </Typography>
              </Box>
            );
          })
        )}
      </Box>
    </Box>
  );
}

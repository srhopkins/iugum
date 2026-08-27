import React, { useState } from "react";
import { Box, Button, Stack, TextField, Typography } from "@mui/material";

function formatTime(us) {
  if (us == null || Number.isNaN(Number(us))) {
    return "";
  }
  const n = Number(us);
  const ms = n > 1e14 ? n / 1000 : n > 1e12 ? n / 1000 : n;
  const d = new Date(ms > 1e12 ? ms / 1000 : ms);
  if (Number.isNaN(d.getTime())) {
    return String(us);
  }
  return d.toISOString().replace("T", " ").replace("Z", "");
}

export function LogsQuery() {
  const [expr, setExpr] = useState('{stream="homelab"}');
  const [rows, setRows] = useState([]);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  function run(e) {
    if (e) {
      e.preventDefault();
    }
    setBusy(true);
    const q = new URLSearchParams({ q: expr, limit: "200" });
    fetch(`/query/logs?${q}`)
      .then(async (res) => {
        const body = await res.json().catch(() => ({}));
        if (!res.ok) {
          throw new Error(body.error || res.statusText);
        }
        return body.logs || [];
      })
      .then((logs) => {
        setRows(logs);
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
        LogQL subset. Example: {`{stream="homelab"} |= "fan"`}
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
          label="LogQL"
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
            No lines. Run a query.
          </Typography>
        ) : (
          rows.map((row, i) => (
            <Box key={`${row.TimeUS || 0}-${i}`} sx={{ mb: 0.5 }}>
              <Typography variant="caption" color="text.secondary">
                {formatTime(row.TimeUS)} {row.Stream || ""} {row.Level || ""}
              </Typography>
              <Typography variant="body2" sx={{ fontFamily: "inherit" }}>
                {row.Message || ""}
              </Typography>
            </Box>
          ))
        )}
      </Box>
    </Box>
  );
}

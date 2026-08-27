import React from "react";
import { Box, Typography } from "@mui/material";

function logEntries(queryResults) {
  const entries = [];
  for (const result of queryResults || []) {
    const data = result && result.data;
    const fromLogs = data && data.logs && data.logs.entries;
    const direct = data && data.entries;
    const list = fromLogs || direct || [];
    for (const entry of list) {
      entries.push(entry);
    }
  }
  entries.sort((a, b) => (b.timestamp || 0) - (a.timestamp || 0));
  return entries;
}

function formatTime(timestamp) {
  if (timestamp == null || Number.isNaN(Number(timestamp))) {
    return "";
  }
  const n = Number(timestamp);
  const ms = n > 1e12 ? n / 1e6 : n > 1e10 ? n : n * 1000;
  const d = new Date(ms);
  if (Number.isNaN(d.getTime())) {
    return "";
  }
  return d.toISOString().replace("T", " ").replace("Z", "");
}

export function LogsTablePanel(props) {
  const spec = props.spec || {};
  const showTime = spec.showTime !== false;
  const allowWrap = spec.allowWrap !== false;
  const height = props.contentDimensions ? props.contentDimensions.height : 240;
  const entries = logEntries(props.queryResults);

  return (
    <Box
      sx={{
        height,
        overflow: "auto",
        fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
        fontSize: 12,
        px: 1,
        py: 0.5,
      }}
    >
      {entries.length === 0 ? (
        <Typography variant="body2" color="text.secondary">
          No log lines for this range.
        </Typography>
      ) : (
        entries.map((entry, i) => (
          <Box
            key={`${entry.timestamp || 0}-${i}`}
            sx={{
              display: "flex",
              gap: 1,
              whiteSpace: allowWrap ? "pre-wrap" : "pre",
              borderBottom: "1px solid",
              borderColor: "divider",
              py: 0.25,
            }}
          >
            {showTime ? (
              <Box component="span" sx={{ color: "text.secondary", flex: "0 0 22ch" }}>
                {formatTime(entry.timestamp)}
              </Box>
            ) : null}
            <Box component="span" sx={{ flex: 1 }}>
              {entry.line || ""}
            </Box>
          </Box>
        ))
      )}
    </Box>
  );
}

export const LogsTable = {
  PanelComponent: LogsTablePanel,
  supportedQueryTypes: ["LogQuery"],
  createInitialOptions: () => ({ allowWrap: true, showTime: true }),
};

export function getPluginModule() {
  return {
    kind: "PluginModule",
    metadata: { name: "iugum-logs-table", version: "0.0.1" },
    spec: {
      plugins: [
        {
          kind: "Panel",
          spec: {
            display: { name: "Logs Table" },
            name: "LogsTable",
          },
        },
      ],
    },
  };
}

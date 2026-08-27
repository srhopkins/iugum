import React from "react";
import { Box, ToggleButton, ToggleButtonGroup } from "@mui/material";
import { LogsQuery } from "./logs-query.jsx";
import { MetricsQuery } from "./metrics-query.jsx";

export function Explore({ lang, onLang }) {
  return (
    <Box>
      <ToggleButtonGroup
        exclusive
        size="small"
        value={lang}
        onChange={(_, v) => {
          if (v) {
            onLang(v);
          }
        }}
        sx={{ mb: 2 }}
      >
        <ToggleButton value="promql">PromQL</ToggleButton>
        <ToggleButton value="logql">LogQL</ToggleButton>
      </ToggleButtonGroup>
      {lang === "promql" ? <MetricsQuery /> : <LogsQuery />}
    </Box>
  );
}

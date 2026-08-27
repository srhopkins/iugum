import React, { useEffect, useState } from "react";
import { Box, List, ListItemButton, ListItemText, Tab, Tabs, Typography } from "@mui/material";
import { Explore } from "./explore.jsx";

export function Home({ tab, onTab, exploreLang, onExploreLang, onOpen }) {
  const [rows, setRows] = useState([]);
  const [err, setErr] = useState("");

  useEffect(() => {
    fetch("/dashboards")
      .then((res) => {
        if (!res.ok) {
          throw new Error("list failed");
        }
        return res.json();
      })
      .then((data) => {
        setRows(data.dashboards || []);
        setErr("");
      })
      .catch((e) => {
        setErr(e.message || "list failed");
      });
  }, []);

  return (
    <Box>
      <Tabs value={tab} onChange={(_, v) => onTab(v)} sx={{ mb: 2 }}>
        <Tab value="dashboards" label="Dashboards" />
        <Tab value="explore" label="Explore" />
      </Tabs>
      {tab === "explore" ? (
        <Explore lang={exploreLang} onLang={onExploreLang} />
      ) : (
        <Box>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
            Boards in the iugum data folder. Click one to open it.
          </Typography>
          {err ? (
            <Typography color="error">{err}</Typography>
          ) : (
            <List>
              {rows.map((row) => (
                <ListItemButton
                  key={row.file}
                  onClick={() => onOpen(row.file)}
                >
                  <ListItemText primary={row.title || row.file} secondary={row.file} />
                </ListItemButton>
              ))}
            </List>
          )}
          {rows.length === 0 && !err ? (
            <Typography variant="body2" color="text.secondary">
              No dashboard files yet.
            </Typography>
          ) : null}
        </Box>
      )}
    </Box>
  );
}

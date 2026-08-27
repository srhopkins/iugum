import React, { useEffect, useState } from "react";
import { Button, Dialog, DialogActions, DialogContent, DialogTitle, Typography } from "@mui/material";

export function DashboardJsonEdit({ dashboard, path, onSaved }) {
  const [open, setOpen] = useState(false);
  const [text, setText] = useState("");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (open) {
      setText(JSON.stringify(dashboard, null, 2));
      setError("");
    }
  }, [open, dashboard]);

  function save() {
    let parsed;
    try {
      parsed = JSON.parse(text);
    } catch (err) {
      setError(err.message || "invalid json");
      return;
    }
    setSaving(true);
    fetch(path || "/homelab-dashboard.json", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(parsed),
    })
      .then(async (res) => {
        const body = await res.text();
        if (!res.ok) {
          throw new Error(body || res.statusText);
        }
        return parsed;
      })
      .then((saved) => {
        onSaved(saved);
        setOpen(false);
      })
      .catch((err) => {
        setError(err.message || "save failed");
      })
      .finally(() => {
        setSaving(false);
      });
  }

  return (
    <>
      <Button variant="outlined" size="small" onClick={() => setOpen(true)}>
        Edit JSON
      </Button>
      <Dialog open={open} onClose={() => setOpen(false)} fullWidth maxWidth="md">
        <DialogTitle>Dashboard JSON</DialogTitle>
        <DialogContent>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
            Saved to the iugum data folder. The board reloads without a rebuild.
          </Typography>
          <textarea
            value={text}
            onChange={(e) => setText(e.target.value)}
            spellCheck={false}
            className="dashboard-json"
          />
          {error ? (
            <Typography variant="body2" color="error" sx={{ mt: 1 }}>
              {error}
            </Typography>
          ) : null}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setOpen(false)}>Cancel</Button>
          <Button onClick={save} disabled={saving} variant="contained">
            Save
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
}

---
description: Cycle editor width (narrow / comfort / wide / full) via top-bar button
---

# Editor width

No plug needed - SilverBullet's `--editor-width` CSS variable. This page adds a **4-step cycle** (top-bar button) persisted in `clientStore`.

| Step | Width | Use |
|---|---|---|
| `narrow` | 720px | prose |
| `comfort` | 900px | Notion default |
| `wide` | 1280px | tables / inventory |
| `full` | min(1600px, 96%) | almost edge-to-edge |

Click the **columns** icon in the top bar (or run `Editor Width: Cycle`). Preference survives reload.

```space-style
/* priority: 20 - wins over Notion's static width */
html[data-editor-width="narrow"] {
  --editor-width: 720px !important;
}
html[data-editor-width="comfort"],
html:not([data-editor-width]) {
  --editor-width: 900px !important;
}
html[data-editor-width="wide"] {
  --editor-width: 1280px !important;
}
html[data-editor-width="full"] {
  --editor-width: min(1600px, 96%) !important;
}
```

```space-lua
-- priority: 20
local WIDTH_KEY = "editorWidth"
local ORDER = { "narrow", "comfort", "wide", "full" }
local LABELS = {
  narrow = "Narrow (720)",
  comfort = "Comfort (900)",
  wide = "Wide (1280)",
  full = "Full (~96%)",
}

local function applyWidth(name)
  if not name or name == "" then
    name = "comfort"
  end
  local ok = false
  for _, w in ipairs(ORDER) do
    if w == name then
      ok = true
      break
    end
  end
  if not ok then
    name = "comfort"
  end
  js.window.document.documentElement.setAttribute("data-editor-width", name)
  return name
end

local function currentWidth()
  local stored = clientStore.get(WIDTH_KEY)
  if stored and stored ~= "" then
    return stored
  end
  return "comfort"
end

local function cycleWidth()
  local cur = currentWidth()
  local idx = 1
  for i, w in ipairs(ORDER) do
    if w == cur then
      idx = i
      break
    end
  end
  local next = ORDER[(idx % #ORDER) + 1]
  clientStore.set(WIDTH_KEY, next)
  applyWidth(next)
  editor.flashNotification("Editor width: " .. LABELS[next])
end

command.define {
  name = "Editor Width: Cycle",
  key = "Ctrl-Alt-w",
  run = function()
    cycleWidth()
  end
}

command.define {
  name = "Editor Width: Comfort",
  run = function()
    clientStore.set(WIDTH_KEY, "comfort")
    applyWidth("comfort")
    editor.flashNotification("Editor width: " .. LABELS.comfort)
  end
}

command.define {
  name = "Editor Width: Wide",
  run = function()
    clientStore.set(WIDTH_KEY, "wide")
    applyWidth("wide")
    editor.flashNotification("Editor width: " .. LABELS.wide)
  end
}

command.define {
  name = "Editor Width: Full",
  run = function()
    clientStore.set(WIDTH_KEY, "full")
    applyWidth("full")
    editor.flashNotification("Editor width: " .. LABELS.full)
  end
}

actionButton.define {
  icon = "columns-2",
  description = "Cycle editor width (narrow -> comfort -> wide -> full)",
  run = function()
    cycleWidth()
  end
}

event.listen {
  name = "editor:pageLoaded",
  run = function()
    applyWidth(currentWidth())
  end
}

event.listen {
  name = "system:ready",
  run = function()
    applyWidth(currentWidth())
  end
}
```

---
description: Cycle the editor width (narrow / comfort / wide / full) from a top-bar button
tags: meta
---

# Editor width

SilverBullet sets the content column with the `--editor-width` CSS variable.
This page adds a four-step cycle on a top-bar button, and remembers the step per
browser in `clientStore`.

| Step | Width | Use |
|---|---|---|
| `narrow` | 720px | prose |
| `comfort` | 900px | the common default |
| `wide` | 1280px | tables, and the Atomdown board |
| `full` | min(1600px, 96%) | almost edge to edge |

Press the **columns** icon in the top bar, or run `Editor Width: Cycle`
(`Ctrl-Alt-W`). The step survives a reload.

A space that never presses the button keeps SilverBullet's own width: the rules
below apply only when a step is set. The `priority: 20` comment puts this sheet
after a theme, so a theme that sets `--editor-width` does not win over a step you
chose.

This page ships in the iugum program, at `Library/Atomdown/Editor Width`. To
change it, make a page of that same name in your own space. A page on disk wins
over the built-in copy.

```space-style
/* priority: 20 */
html[data-editor-width="narrow"] {
  --editor-width: 720px !important;
}
html[data-editor-width="comfort"] {
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

-- applyWidth puts the step on the html element, where the space-style rules
-- above select on it. An unknown or absent step removes the attribute, so the
-- space keeps SilverBullet's own width.
local function applyWidth(name)
  local root = js.window.document.documentElement
  for _, w in ipairs(ORDER) do
    if w == name then
      root.setAttribute("data-editor-width", name)
      return name
    end
  end
  root.removeAttribute("data-editor-width")
  return nil
end

local function currentWidth()
  local stored = clientStore.get(WIDTH_KEY)
  if stored and stored ~= "" then
    return stored
  end
  return nil
end

local function cycleWidth()
  local cur = currentWidth()
  local idx = 0
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

local function setWidth(name)
  clientStore.set(WIDTH_KEY, name)
  applyWidth(name)
  editor.flashNotification("Editor width: " .. LABELS[name])
end

command.define {
  name = "Editor Width: Cycle",
  key = "Ctrl-Alt-w",
  run = cycleWidth,
}

command.define {
  name = "Editor Width: Narrow",
  run = function() setWidth("narrow") end,
}

command.define {
  name = "Editor Width: Comfort",
  run = function() setWidth("comfort") end,
}

command.define {
  name = "Editor Width: Wide",
  run = function() setWidth("wide") end,
}

command.define {
  name = "Editor Width: Full",
  run = function() setWidth("full") end,
}

actionButton.define {
  icon = "columns-2",
  description = "Cycle the editor width (narrow, comfort, wide, full)",
  run = cycleWidth,
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

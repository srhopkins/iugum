---
description: Choose Day, Night, or System appearance from the top menu
---

Click the sun icon to choose **Day**, **Night**, or **System**. System follows
changes to your device appearance. The choice is saved for this browser using
SilverBullet's native theme preference. This extension needs no chat or search plug.

```space-lua
local function applyFamily()
  js.window.document.documentElement.setAttribute("data-iugum-theme", clientStore.get("iugumTheme") or "default")
end
local function chooseFamily()
  local selected = editor.filterBox("Theme family", {
    {name = "Default", description = "Blue accents and prompt cards"},
    {name = "Warm neutral", description = "Cream and charcoal with amber accents"}
  }, "Choose a theme; appearance mode stays unchanged", "Theme")
  if not selected then return end
  clientStore.set("iugumTheme", selected.name == "Warm neutral" and "warm" or "default")
  applyFamily()
end
local function chooseTheme()
  local selected = editor.filterBox("Appearance", {
    {name = "Day", description = "Always use the light theme"},
    {name = "Night", description = "Always use the dark theme"},
    {name = "System", description = "Follow your device appearance"},
    {name = "Theme family…", description = "Default or Warm neutral"}
  }, "Choose Day, Night, or System", "Theme")
  if not selected then return end
  if selected.name == "Theme family…" then chooseFamily() return end
  if selected.name == "System" then
    clientStore.delete("darkMode")
  else
    clientStore.set("darkMode", selected.name == "Night")
  end
  editor.reloadUI()
end

command.define { name = "Appearance: Choose family", run = chooseFamily }
event.listen { name = "editor:pageLoaded", run = applyFamily }
event.listen { name = "system:ready", run = applyFamily }
applyFamily()

command.define { name = "Appearance: Choose theme", run = chooseTheme }
actionButton.define {
  icon = "sun",
  description = "Appearance: Day, Night, or System",
  run = chooseTheme
}
```

```space-style
html[data-theme="dark"] {
  --iugum-chat-prompt-background: #223d72;
  --iugum-chat-prompt-color: #f7fafe;
}
```


Theme family is independent of Day/Night/System. Choose **Theme family…** from
Appearance, or run **Appearance: Choose family**. It persists for this browser.
The warm palette changes shared values only; feature components do not branch on it.

```space-style
html[data-iugum-theme="warm"][data-theme] {
  --root-background-color: #faf6ed;
  --root-color: #332c24;
  --top-background-color: #eee5d5;
  --top-color: #332c24;
  --top-border-color: #cdbfa7;
  --link-color: #80551b;
  --ui-accent-color: #80551b;
  --ui-accent-contrast-color: #fffaf0;
  --subtle-color: #746653;
  --subtle-background-color: #eae1d2;
  --ui-surface-border-color: #cdbfa7;
  --iugum-chat-prompt-background: #eee0c4;
  --iugum-chat-prompt-color: #493413;
  --iugum-chat-prompt-border: 1px solid #baa078;
  --iugum-chat-prompt-radius: 3px;
}
html[data-iugum-theme="warm"][data-theme="dark"] {
  --root-background-color: #25231f;
  --root-color: #eee6d8;
  --top-background-color: #302c25;
  --top-color: #eee6d8;
  --top-border-color: #625542;
  --link-color: #e0ba78;
  --ui-accent-color: #d3ac69;
  --ui-accent-contrast-color: #282219;
  --subtle-color: #b9ad99;
  --subtle-background-color: #393329;
  --ui-surface-border-color: #625542;
  --iugum-chat-prompt-background: #483923;
  --iugum-chat-prompt-color: #f3e2c3;
  --iugum-chat-prompt-border: 1px solid #9c8053;
}
html[data-iugum-theme="warm"][data-theme] {
  --panel-background-color: var(--root-background-color);
  --panel-border-color: var(--ui-surface-border-color);
  --modal-background-color: var(--root-background-color);
  --modal-color: var(--root-color);
  --modal-border-color: var(--ui-surface-border-color);
  --modal-help-background-color: var(--subtle-background-color);
  --modal-help-color: var(--subtle-color);
  --modal-description-color: var(--subtle-color);
  --button-background-color: var(--subtle-background-color);
  --button-color: var(--root-color);
  --button-border-color: var(--ui-surface-border-color);
  --action-button-color: var(--subtle-color);
  --action-button-hover-color: var(--link-color);
  --iugum-focus-color: var(--link-color);
}
```

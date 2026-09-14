# Wiki body width

Copy `EditorWidth.md` into your SilverBullet space (for example,
`Library/Styles/EditorWidth.md`). SilverBullet indexes its standard Space Lua
and Space Style blocks. The columns icon cycles Comfort, Wide, and Full;
the selection is saved in SilverBullet clientStore. Ctrl-Alt-W also cycles it.

This extension uses SilverBullet's `--editor-width` variable and actionButton API.
It does not require agent chat or search. Remove the page to remove the extension.

Regression checks: `scripts/chief-fe-check.sh` tests the three widths, reload
persistence, and independent chat resizing, plus chat pinning, fades and expansion.

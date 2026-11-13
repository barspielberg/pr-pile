# Real compiled app, live public cli/cli and astral-sh/uv data. Run from the repository root:
#   make build
#   vhs docs/assets/demo.tape
# Requires gh authentication, ttyd, ffmpeg and sandbox-capable Chromium.
# The hidden startup allows API requests and badge scripts to finish.
# Inspect the poster after each capture: PR contents/order change over time.
Output docs/public/demo.gif
Output docs/public/demo.mp4

Set Shell "bash"
Set Theme "Catppuccin Mocha"
Set FontFamily "IBM Plex Mono"
Set FontSize 20
Set Columns 106
Set Rows 19
Set Padding 20
Set Framerate 10
Set TypingSpeed 80ms

Env TERM "xterm-256color"
Env COLORTERM "truecolor"
Env NO_COLOR ""
Env CLICOLOR_FORCE "1"

Hide
Type "PILE_CONFIG=docs/assets/demo-config.yml ./pile"
Enter
Sleep 25s
# Start and end at the first PR, with all three sections visible.
# Only Needs review has badges; Hash policies has one complete two-PR chain.
Type "j"
Show
Screenshot docs/public/demo-poster.png
Sleep 3s

# Section navigation, then return to the first PR.
Type "l"
Sleep 2s
Type "l"
Sleep 2s
Type "h"
Sleep 1s
Type "h"
Type "j"
Sleep 2s

# Select just two PRs, then clear the selection without invoking an action.
Space
Type "j"
Space
Sleep 3s
Escape

# Inspect labels and badge results on a live external PR.
Type "d"
Sleep 3s
Type "G"
Sleep 1500ms
Escape

# Search highlights in place. The query also matches the left badge.
Type "/"
Type "trunk"
Sleep 3s
Enter
Sleep 1s
Escape

# Show the reviewed PR, including live labels/reviewers when available.
Type "l"
Type "j"
Type "d"
Sleep 3s
Type "G"
Sleep 3s
Escape

# Return to the opening composition for a quiet loop.
Type "g"
Type "j"
Sleep 3s
Hide
Type "q"

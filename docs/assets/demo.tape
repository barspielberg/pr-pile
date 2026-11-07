# Regenerates the demo recording for the README and the docs site.
#
# Run from the repo root:
#   make docs-assets
#   # or: vhs docs/assets/demo.tape
#
# The capture is the real board against a public repo with real open PRs
# (docs/assets/demo-config.yml points at cli/cli), so the recording cannot
# drift from the UI. It is generated, not hand-recorded.

Output docs/public/demo.gif
Output docs/public/demo.mp4

Set Shell "bash"
Set Theme "Catppuccin Mocha"
Set FontFamily "IBM Plex Mono"
Set FontSize 20
Set Columns 90
Set Rows 24
Set Padding 20
Set Framerate 30
Set TypingSpeed 30ms

# Ensure CI/agent shells cannot disable the terminal palette for the capture.
Env TERM "xterm-256color"
Env COLORTERM "truecolor"
Env NO_COLOR ""
Env CLICOLOR_FORCE "1"

Hide
Type "PILE_CONFIG=docs/assets/demo-config.yml ./pile"
Enter
Sleep 15s
Show
Screenshot docs/public/demo-poster.png
Sleep 2s

# Walk the board: j/k move, one line per press.
Type "j"
Sleep 650ms
Type "j"
Sleep 650ms
Type "j"
Sleep 650ms
Type "j"
Sleep 1s

# / searches the drawn board without moving it.
Type "/"
Type "fix"
Sleep 2500ms

# End on the board so the loop never flashes the shell.
Escape
Sleep 1500ms
Hide
Type "q"

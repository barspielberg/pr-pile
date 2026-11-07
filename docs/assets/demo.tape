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
Set FontSize 18
Set Columns 96
Set Rows 28
Set Padding 0
Set Framerate 15
Set TypingSpeed 30ms

Hide
Type "PILE_CONFIG=docs/assets/demo-config.yml ./pile"
Enter
Sleep 15s
Show

# Walk the board: j/k move, one line per press.
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms

# / searches the drawn board without moving it.
Type "/"
Type "fix"
Sleep 1500ms

# esc clears the highlight, q quits.
Escape
Sleep 500ms
Type "q"

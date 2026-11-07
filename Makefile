build:
	go build -o pile ./cmd/pile

test:
	go test ./... -short

check: build test
	go vet ./...

# run is the dev loop: build and run this checkout without go install, so the
# pile already on PATH keeps working while you try a change.
run: build
	./pile

# install is the other end of that: put this checkout on PATH as the real pile.
# Gated on check, because the binary you type `pile` for should be one that
# built, passed and vetted.
install: check
	go install ./cmd/pile

# docs-assets regenerates the demo recording (GIF + MP4) and the OG image from
# their sources, so neither can drift from the UI or the wordmark. It builds
# ./pile first because docs/assets/demo.tape runs the real tool.
#
# Requires vhs (github.com/charmbracelet/vhs), ttyd and ffmpeg on PATH, and
# ImageMagick for the OG image. On Linux, headless Chrome may need
# VHS_NO_SANDBOX=1. Run from the repo root.
docs-assets: build
	vhs docs/assets/demo.tape
	convert -background none docs/assets/og.svg docs/public/og.png

.PHONY: build test check run install docs-assets

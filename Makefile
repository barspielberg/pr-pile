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

.PHONY: build test check run install

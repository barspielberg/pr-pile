build:
	go build -o prs ./cmd/prs

test:
	go test ./... -short

check: build test
	go vet ./...

# run is the dev loop: build and run this checkout without go install, so the
# prs already on PATH keeps working while you try a change.
run: build
	./prs

# install is the other end of that: put this checkout on PATH as the real prs.
# Gated on check, because the binary you type `prs` for should be one that
# built, passed and vetted.
install: check
	go install ./cmd/prs

.PHONY: build test check run install

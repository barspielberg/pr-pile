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

.PHONY: build test check run

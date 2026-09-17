build:
	go build -o prs ./cmd/prs

test:
	go test ./... -short

check: build test
	go vet ./...

.PHONY: build test check

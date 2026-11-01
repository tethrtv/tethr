GOLANGCI_LINT := github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0

.PHONY: run test lint

run:
	go run ./cmd/tethr

test:
	go test -race ./...

lint:
	go run $(GOLANGCI_LINT) run

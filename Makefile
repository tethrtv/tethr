GOLANGCI_LINT := github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
SQLC := github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1

.PHONY: run test lint gen migrate

run:
	go run ./cmd/tethr

test:
	go test -race ./...

lint:
	go run $(GOLANGCI_LINT) run

gen:
	go run $(SQLC) generate

migrate:
	DATABASE_URL=postgres://tethr:tethr@localhost:5432/tethr?sslmode=disable go run ./cmd/tethr migrate

GOLANGCI_LINT := github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
SQLC := github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1
OAPI_CODEGEN := github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0

.PHONY: run test lint gen migrate

run:
	go run ./cmd/tethr

test:
	go test -race ./...

lint:
	go run $(GOLANGCI_LINT) run

gen:
	go run $(SQLC) generate
	go run $(OAPI_CODEGEN) -config internal/api/oapi.yaml spec/tethr/openapi.yaml

migrate:
	go run ./cmd/tethr migrate

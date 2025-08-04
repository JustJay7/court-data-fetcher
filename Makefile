.PHONY: build run test clean docker-build docker-run lint fmt

BINARY_NAME=court-data-fetcher
DOCKER_IMAGE=court-data-fetcher:latest

build:
	go build -o $(BINARY_NAME) cmd/server/main.go

run:
	go run cmd/server/main.go

test:
	go test -v ./...

test-coverage:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

clean:
	go clean
	rm -f $(BINARY_NAME)
	rm -f coverage.out

docker-build:
	docker build -t $(DOCKER_IMAGE) .

docker-run:
	docker-compose up --build

docker-stop:
	docker-compose down

lint:
	golangci-lint run

fmt:
	go fmt ./...

deps:
	go mod download
	go mod tidy

migrate:
	go run cmd/server/main.go migrate

dev:
	air -c .air.toml
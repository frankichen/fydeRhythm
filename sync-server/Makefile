.PHONY: test run build docker-up docker-down token

test:
	go test ./...

run:
	go run ./cmd/server

build:
	CGO_ENABLED=0 go build -trimpath -o bin/fyderhythm-sync ./cmd/server

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down

token:
	@openssl rand -hex 32

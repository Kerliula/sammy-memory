.PHONY: db-up db-down migrate run worker generate test lint

db-up:
	docker compose up -d --wait

db-down:
	docker compose down

migrate:
	go run ./cmd/sammy migrate

run:
	go run ./cmd/sammy serve

worker:
	go run ./cmd/sammy worker

generate:
	sqlc generate

test:
	go test ./...

lint:
	golangci-lint run ./...

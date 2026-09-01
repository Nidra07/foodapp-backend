.PHONY: run build test lint sqlc-generate migrate-up migrate-down docker-up docker-down tidy

run:
	go run ./cmd/api

build:
	go build -o bin/api ./cmd/api

test:
	go test ./... -race -cover

lint:
	golangci-lint run ./...

tidy:
	go mod tidy

# Requires: go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
sqlc-generate:
	sqlc generate

# Requires: go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
migrate-up:
	migrate -path db/migrations -database "postgres://foodapp:foodapp_local_pw@localhost:5432/foodapp?sslmode=disable" up

migrate-down:
	migrate -path db/migrations -database "postgres://foodapp:foodapp_local_pw@localhost:5432/foodapp?sslmode=disable" down 1

migrate-new:
	@read -p "migration name: " name; \
	migrate create -ext sql -dir db/migrations -seq $$name

docker-up:
	docker compose up -d

docker-down:
	docker compose down

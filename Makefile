DB_URL := postgresql://siemens_admin:siemens_password_123@localhost:5432/siemens_reservation?sslmode=disable
MIGRATIONS_DIR := ./internal/database/postgres/migrations
SEEDS_DIR := ./internal/database/postgres/seeds

.PHONY: migrate-up migrate-down migrate-status migrate-create

migrate-up:
	goose -dir $(MIGRATIONS_DIR) postgres $(DB_URL) up

migrate-down:
	goose -dir $(MIGRATIONS_DIR) postgres $(DB_URL) down

migrate-down-all:
	goose -dir $(MIGRATIONS_DIR) postgres $(DB_URL) down-to 0

migrate-status:
	goose -dir $(MIGRATIONS_DIR) postgres $(DB_URL) status

migrate-create:
	@read -p "Enter migration name: " name; \
	goose -s -dir $(MIGRATIONS_DIR) create $$name sql

migrate-reset:
	goose -dir $(MIGRATIONS_DIR) postgres $(DB_URL) reset


run:
	@templ generate
	@go run cmd/main.go

dev:
	@air

css-watch:
	@npx tailwindcss -i ./assets/input.css -o ./assets/output.css --watch

templ:
	@templ generate

build:
	@templ generate
	@go build -o ./bin/server ./cmd/main.go
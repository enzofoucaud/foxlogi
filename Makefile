BINARY  := foxlogi
IMAGE   := foxlogi:latest
COMPOSE := docker compose -f docker/docker-compose.yml

.PHONY: help run build test vet fmt tidy clean \
        docker-build docker-up docker-down docker-logs docker-restart

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

## --- Local development ---

run: ## Run the bot locally (reads .env)
	go run .

build: ## Build the binary
	go build -ldflags="-s -w" -o $(BINARY) .

test: ## Run all tests
	go test ./...

vet: ## Run go vet
	go vet ./...

fmt: ## Format the code
	go fmt ./...

tidy: ## Tidy go.mod / go.sum
	go mod tidy

clean: ## Remove build artifacts and local database files
	rm -f $(BINARY) *.db *.db-wal *.db-shm

## --- Docker / deployment ---

docker-build: ## Build the Docker image
	$(COMPOSE) build

docker-up: ## Start the bot in the background
	$(COMPOSE) up -d

docker-down: ## Stop and remove the container
	$(COMPOSE) down

docker-restart: ## Rebuild and restart the bot
	$(COMPOSE) up -d --build

docker-logs: ## Follow the container logs
	$(COMPOSE) logs -f

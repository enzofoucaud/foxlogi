BINARY  := foxlogi
IMAGE   := foxlogi:latest
COMPOSE := docker compose -f docker/docker-compose.yml
COMPOSE_PROD := docker compose -f docker/docker-compose.prod.yml

.PHONY: help run build test vet fmt tidy clean \
        docker-build docker-up docker-down docker-logs docker-restart \
        prod-up prod-down prod-logs prod-pull prod-build prod-deploy

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

## --- Production ---

prod-up: ## Start the bot in production (uses the existing image; does NOT rebuild)
	$(COMPOSE_PROD) up -d

prod-build: ## Rebuild the production image from the current source
	$(COMPOSE_PROD) build

prod-deploy: ## Rebuild from source and (re)start — use this for on-box deploys
	$(COMPOSE_PROD) up -d --build

prod-down: ## Stop and remove the production container
	$(COMPOSE_PROD) down

prod-pull: ## Pull the pinned production image (set FOXLOGI_IMAGE)
	$(COMPOSE_PROD) pull

prod-logs: ## Follow the production container logs
	$(COMPOSE_PROD) logs -f

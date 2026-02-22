.PHONY: run build tidy

# Copy .env.example to .env if it doesn't exist, then load env vars and run
run:
	@if [ ! -f .env ]; then cp .env.example .env && echo "Created .env from .env.example — fill in your API keys before running!"; exit 1; fi
	@set -a && . ./.env && set +a && GOTOOLCHAIN=local go run .

build:
	GOTOOLCHAIN=local go build -o bin/linguaai .

tidy:
	GOTOOLCHAIN=local go mod tidy

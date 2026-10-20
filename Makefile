.DEFAULT_GOAL := build

PORT ?= 8080
DATABASE_PATH ?= ./game.db
export PORT DATABASE_PATH

.PHONY: build run test deploy

build:
	go build -o server ./cmd/server

run:
	go run ./cmd/server

test:
	@files=$$(gofmt -l .) || exit $$?; \
	if [ -n "$$files" ]; then printf 'Run gofmt -w on:\n%s\n' "$$files"; exit 1; fi
	go vet ./...
	go test -race ./...

deploy:
	@test -n "$$RENDER_DEPLOY_HOOK_URL" || { echo "Set RENDER_DEPLOY_HOOK_URL from Render service Settings." >&2; exit 1; }
	@curl --fail --silent --show-error --connect-timeout 10 --max-time 60 \
		--request POST "$$RENDER_DEPLOY_HOOK_URL"
	@printf '\nDeployment requested; check Render for completion.\n'

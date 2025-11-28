# Include variables from the .envrc file
include .envrc

# ================================================================================================== #
# HELPERS
# ================================================================================================== #

## help: print this help message
.PHONY: help
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'

# ================================================================================================== #
# DEVELOPMENT
# ================================================================================================== #

## run/api: run the cmd/api application
.PHONY: run/api
run/api:
	@go run ./cmd/api -db-dsn=${MOVIE_CATALOGUE}

## db/psql: connect to the database using psql
.PHONY: db/psql
db/psql:
	psql ${MOVIE_CATALOGUE}

## db/migrations/new name=$1: create a new database migration
.PHONY: db/migrations/new
db/migrations/new:
	@echo 'Creating migration files for ${name}...'
	migrate create -seq -ext=.sql -dir=./migrations ${name}

## db/migrations/up: apply all up database migrations
.PHONY: db/mig/up
db/mig/up:
	@echo 'Running up migrations...'
	migrate -path ./migrations -database ${MOVIE_CATALOGUE} up

## db/migrations/down: apply all up database migrations
.PHONY: db/mig/down
db/mig/down:
	@echo 'Running down migrations...'
	migrate -path ./migrations -database ${MOVIE_CATALOGUE} down

# ================================================================================================== #
# QUALITY CONTROL
# ================================================================================================== #

## tidy: format all .go files and tidy module dependencies
.PHONY: tidy
tidy:
	@echo 'Formatting .go files...'
	go fmt ./...
	@echo 'Tidying module dependencies...'
	go mod tidy
	@echo 'Verifying and vendoring module dependencies...'
	go mod verify
	go mod vendor

## audit: run quality control checks
.PHONY: audit
audit:
	@echo 'Checking module dependencies'
	go mod tidy -diff
	go mod verify
	@echo 'Vetting code...'
	go vet ./...
	staticcheck ./...
	@echo 'Running tests...'
	go test -race -vet=off ./...

# ================================================================================================== #
# BUILD
# ================================================================================================== #

.PHONY: build/api
build/api:
	@echo 'Building cmd/api...'
	go build -ldflags='s' -o=./bin/api ./cmd/api

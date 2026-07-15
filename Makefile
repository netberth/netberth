.PHONY: build run dev docker-build docker-up clean web-build

APP=netberth
GO=go
NPM=cd web && npm

build:
	CGO_ENABLED=1 $(GO) build -ldflags="-s -w" -o bin/$(APP) ./cmd/$(APP)

run: build
	NH_JWT_SECRET=$$(openssl rand -base64 48) NH_LOG_LEVEL=info ./bin/$(APP)

dev:
	NH_LOG_FORMAT=console NH_JWT_SECRET=dev-secret-do-not-use-in-production \
		$(GO) run ./cmd/$(APP)

web-build:
	$(NPM) install && $(NPM) run build

web-dev:
	$(NPM) install && $(NPM) run dev

docker-build:
	docker build -t netberth:latest .

docker-up:
	docker compose up -d

docker-down:
	docker compose down

clean:
	rm -rf bin/ dist/

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

test:
	$(GO) test ./... -v -race -count=1

test-cover:
	$(GO) test ./... -v -race -coverprofile=coverage.out
	$(GO) tool cover -html=coverage.out -o coverage.html

all: web-build build

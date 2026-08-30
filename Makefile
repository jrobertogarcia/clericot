.PHONY: dev test lint generate migrate-up clean

dev:
	docker compose up -d

test:
	go test -v -race ./...

lint:
	golangci-lint run ./...

generate:
	sqlc generate

clean:
	docker compose down -v

.PHONY: check test web build dev clean

check: web
	go vet ./...
	go test ./...

test:
	go test ./...

web:
	@if [ -d web ]; then cd web && npx tsc --noEmit && npx vite build; fi

build: web
	CGO_ENABLED=0 go build -trimpath -o strapontin .

dev:
	go run . serve

clean:
	rm -rf strapontin web/dist

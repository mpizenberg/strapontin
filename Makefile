.PHONY: check test web build dev clean

check: web
	go vet ./...
	go test ./...

test:
	go test ./...

web:
	cd web && npx tsc --noEmit && npm run build --silent

build: web
	CGO_ENABLED=0 go build -trimpath -o strapontin .

dev:
	go run . serve

clean:
	rm -rf strapontin web/dist/assets web/dist/index.html

.PHONY: build dev test clean

# The client is built into the Go package that embeds it, so `build` always
# produces one self-contained binary.
build:
	cd ui && npm ci --silent && npm run build
	go build -o todo .

# Two processes: the API on 8765, and Vite on 5173 proxying /api to it.
dev:
	@echo "run in two shells:"
	@echo "  go run . serve"
	@echo "  cd ui && npm run dev"

test:
	go vet ./...
	go test ./...
	cd ui && npx tsc --noEmit

clean:
	rm -f todo
	rm -rf ui/node_modules internal/ui/dist/assets

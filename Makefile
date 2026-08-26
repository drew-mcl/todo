# The built client is committed, so a clone needs nothing but Go.
#
#   git clone … && make        →  a working binary, no Node, no Swift
#
# Node is only needed to *change* the client; Swift only for the optional
# macOS capture bar.

.PHONY: all build ui test test-go test-ui dev clean install

all: build

## build: compile the binary from what is in the repo (Go only)
build:
	go build -o todo .

## ui: rebuild the web client, then the binary (needs Node)
ui:
	cd ui && npm ci --silent && npm run build
	go build -o todo .

## install: put it on your PATH
install: build
	go install .

## test: everything
test: test-go test-ui

test-go:
	go vet ./...
	go test ./...

## test-ui: needs Node
test-ui:
	cd ui && npm test && npx tsc --noEmit

## dev: API on 8765, Vite on 5173 proxying to it
dev:
	@echo "two shells:"
	@echo "  go run . serve"
	@echo "  cd ui && npm run dev"

clean:
	rm -f todo
	rm -rf ui/node_modules

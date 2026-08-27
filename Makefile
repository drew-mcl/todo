# The built client is committed, so a clone needs nothing but Go.
#
#   git clone … && make        →  a working binary, no Node, no Swift
#
# Node is only needed to *change* the client; Swift only for the optional
# macOS capture bar.

APP    = mac/build/todo capture.app
BAR_ID = com.drew-mcl.todo.capture
# Every source but the app's own entry point, which a test replaces.
BAR_LIB = $(filter-out mac/Sources/main.swift,$(wildcard mac/Sources/*.swift))

.PHONY: all build ui bar bar-install bar-uninstall test test-go test-ui test-bar dev clean install

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

## bar: build the optional macOS capture bar (needs the Swift toolchain)
bar: build
	@mkdir -p "$(APP)/Contents/MacOS"
	@cp mac/Info.plist "$(APP)/Contents/Info.plist"
	swiftc -O -swift-version 5 \
		-target $$(uname -m)-apple-macos13 \
		-sdk $$(xcrun --show-sdk-path) \
		-framework AppKit -framework Carbon \
		-o "$(APP)/Contents/MacOS/todo-capture" mac/Sources/*.swift
	@cp todo "$(APP)/Contents/MacOS/todo"
	@cp todo mac/build/todo
	@touch "$(APP)"
	@echo "built $(APP) -- open it, then press ⌥Space"

## bar-install: put the bar in ~/Applications and start it at login
bar-install: bar install
	@mkdir -p "$$HOME/Applications"
	@rm -rf "$$HOME/Applications/todo capture.app"
	@cp -R "$(APP)" "$$HOME/Applications/"
	@mkdir -p "$$HOME/Library/LaunchAgents"
	@sed "s|__APP__|$$HOME/Applications/todo capture.app|" mac/LaunchAgent.plist \
		> "$$HOME/Library/LaunchAgents/$(BAR_ID).plist"
	@launchctl unload "$$HOME/Library/LaunchAgents/$(BAR_ID).plist" 2>/dev/null || true
	@launchctl load "$$HOME/Library/LaunchAgents/$(BAR_ID).plist"
	@echo "the capture bar is in the menu bar and starts at login -- ⌥Space opens it"

## bar-uninstall: stop it and take it back off
bar-uninstall:
	@launchctl unload "$$HOME/Library/LaunchAgents/$(BAR_ID).plist" 2>/dev/null || true
	@rm -f "$$HOME/Library/LaunchAgents/$(BAR_ID).plist"
	@rm -rf "$$HOME/Applications/todo capture.app"
	@echo "removed"

## test: everything
test: test-go test-ui

test-go:
	go vet ./...
	go test ./...

## test-ui: needs Node
test-ui:
	cd ui && npm test && npx tsc --noEmit

SWIFT = swiftc -swift-version 5 -target $(shell uname -m)-apple-macos13 \
	-sdk $(shell xcrun --show-sdk-path) -framework AppKit -framework Carbon

## test-bar: the capture bar, against a live bridge (needs Swift)
test-bar: build
	@mkdir -p mac/build
	$(SWIFT) -o mac/build/bar-test \
		mac/Sources/Bridge.swift mac/Sources/Theme.swift mac/Sources/Render.swift \
		mac/Sources/Keys.swift mac/Sources/Vim.swift mac/Tests/main.swift
	@TODO_BIN=$$PWD/todo TODO_DB=$$(mktemp -d)/todo.db mac/build/bar-test

## dev: API on 8765, Vite on 5173 proxying to it
dev:
	@echo "two shells:"
	@echo "  go run . serve"
	@echo "  cd ui && npm run dev"

clean:
	rm -f todo
	rm -rf ui/node_modules mac/build

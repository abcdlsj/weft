# build the panel into web/dist, then the single binary
BINARY := weft
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/abcdlsj/weft/internal/weft.AgentVersion=$(VERSION)

.PHONY: build panel test run demo fmt clean

build: panel
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/weft

panel:
	npm --prefix web ci
	npm --prefix web run build

test:
	go test ./...

demo: build
	./$(BINARY) serve --demo --listen :8787

run: build
	./$(BINARY) serve --listen :8787

fmt:
	gofmt -l -w ./cmd ./internal ./tools

clean:
	rm -f $(BINARY) weft.db

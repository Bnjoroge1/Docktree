set dotenv-load

binary := "docktree"

default: build

build:
    go build -o {{ binary }} ./cmd/docktree

run *args:
    go run ./cmd/docktree {{ args }}

test:
    go test ./internal/...

test-all:
    go test ./...

lint:
    go vet ./...

fmt:
    gofmt -w .

clean:
    rm -f {{ binary }}

install: build
    go install ./cmd/docktree
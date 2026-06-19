set dotenv-load

binary := "docktree"
version := `cat VERSION 2>/dev/null || echo "dev"`

default: build

build:
    go build -ldflags "-X github.com/bnjoroge/docktree/internal/cli.version={{ version }}" -o {{ binary }} ./cmd/docktree

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
    go install -ldflags "-X github.com/bnjoroge/docktree/internal/cli.version={{ version }}" ./cmd/docktree
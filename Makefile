.PHONY: build run test fmt vet clean

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o bin/fbs ./cmd/fbs

run: build
	./bin/fbs

test:
	go test ./...

fmt:
	gofmt -w internal cmd

vet:
	go vet ./...

clean:
	rm -rf bin

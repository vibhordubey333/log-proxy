.PHONY: build run-server run-server-fixture run-client test clean

BINDIR := bin
BUILD ?= lastSuccessfulBuild

build:
	go build -o $(BINDIR)/server ./cmd/server
	go build -o $(BINDIR)/client ./cmd/client

run-server: build
	./$(BINDIR)/server

run-server-fixture: build
	./$(BINDIR)/server --fixture=testdata/console.logs

run-client: build
	./$(BINDIR)/client --build=$(BUILD)

test:
	go test ./... -v

clean:
	rm -rf $(BINDIR) cache
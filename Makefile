# Go environment
GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)
MINER_BINARY := bin/dero-miner-$(GOOS)-$(GOARCH)
ifeq ($(GOOS),windows)
MINER_BINARY := $(MINER_BINARY).exe
endif

.PHONY: all build-miner build-engram clean

all: build-miner build-engram

build-miner:
	@echo "Building miner from source..."
	@mkdir -p bin
	go build -ldflags="-s -w" -o $(MINER_BINARY) github.com/deroproject/derohe/cmd/dero-miner
	@echo "Miner built: $(MINER_BINARY)"

build-engram:
	@echo "Building Engram..."
	go build -ldflags="-s -w" -o engram .
	@echo "Engram built: engram"

clean:
	rm -f bin/dero-miner-*
	rm -f engram
	rm -f engram-linux

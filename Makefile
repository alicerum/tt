# Build, install and clean tt and its calendar helper tt-cal (D40).
# Installed side by side, tt finds tt-cal next to its own binary.
PREFIX ?= $(HOME)/.local
BIN    := bin

.PHONY: build install clean

build:
	mkdir -p $(BIN)
	go build -o $(BIN)/tt ./cmd/tt
	sh helpers/tt-cal/build.sh $(CURDIR)/$(BIN)

install:
	mkdir -p $(PREFIX)/bin
	go build -o $(PREFIX)/bin/tt ./cmd/tt
	sh helpers/tt-cal/build.sh $(PREFIX)/bin

clean:
	rm -rf $(BIN)

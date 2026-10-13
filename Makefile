# Build, install and clean tt and its calendar helper tt-cal (D40).
# Installed side by side, tt finds tt-cal next to its own binary.
PREFIX ?= $(HOME)/.local
CONFIG ?= $(HOME)/.config/tt/config.toml
BIN    := bin

.PHONY: build install config clean

build:
	mkdir -p $(BIN)
	go build -o $(BIN)/tt ./cmd/tt
	sh helpers/tt-cal/build.sh $(CURDIR)/$(BIN)

install:
	mkdir -p $(PREFIX)/bin
	go build -o $(PREFIX)/bin/tt ./cmd/tt
	sh helpers/tt-cal/build.sh $(PREFIX)/bin

# config copies config.example.toml (every default) into place; an
# existing config is left alone.
config:
	@if [ -e "$(CONFIG)" ]; then echo "$(CONFIG) exists; left alone (see config.example.toml)"; \
	else mkdir -p "$(dir $(CONFIG))" && cp config.example.toml "$(CONFIG)" && echo "wrote $(CONFIG)"; fi

clean:
	rm -rf $(BIN)

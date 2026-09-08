NAME = ce-libify
MODULE = github.com/alessiodam/toolchain_libify

PYTHON ?= python
CEDEV ?= $(shell cedev-config --prefix)

BUILD = build
VENDOR = $(BUILD)/vendor

ifeq ($(OS),Windows_NT)
EXE = .exe
else
EXE =
endif

BINARY = $(BUILD)/$(NAME)$(EXE)
FASMG = $(BUILD)/fasmg$(EXE)

GO_SOURCES = $(wildcard *.go cmd/*.go internal/*/*.go)
MACROS = $(wildcard $(VENDOR)/*.inc $(VENDOR)/*.alm)

.PHONY: all build test vet fmt deps install uninstall clean

all: build

build: $(BINARY)

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null)

$(BINARY): $(GO_SOURCES) go.mod go.sum
	$(Q)go build -ldflags "-X main.version=$(VERSION)" -o $@ .

test:
	$(Q)go test ./...

vet:
	$(Q)go vet ./...

fmt:
	$(Q)gofmt -l -w .

deps:
	$(Q)$(PYTHON) get_fasmg.py --bin $(BUILD) --vendor $(VENDOR)

$(FASMG):
	$(Q)$(MAKE) deps

install: build $(FASMG)
	$(Q)$(BINARY) selfinstall --cedev "$(CEDEV)" --binary $(BINARY) --fasmg-source $(FASMG) --vendor $(VENDOR)

uninstall:
	$(Q)$(BINARY) selfinstall --cedev "$(CEDEV)" --remove

clean:
	$(Q)go clean
	$(Q)rm -rf $(BUILD)

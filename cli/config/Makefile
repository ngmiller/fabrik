IMAGE = ngmiller/fabrik:build
GOPATH = /go/src/github.com/ngmiller/fabrik/cli/config

RUN = docker run -it --rm \
	  -v $(HOME)/.aws:/root/.aws \
	  -v $(PWD):$(GOPATH) \
	  -v $(PWD)/.cache:/root/.cache/go-build \
	  -w $(GOPATH) \
	  $(IMAGE)

BIN_MACOS = fabrik-config-macos
BIN_LINUX = fabrik-config-linux

COMPILE_MACOS = env GOOS=darwin go build -ldflags="-s -w"
COMPILE_LINUX = env GOOS=linux go build -ldflags="-s -w"

.PHONY: image
image:
	@docker build -t $(IMAGE) .

.PHONY: deps
deps:
	@$(RUN) dep ensure

.PHONY: build
build:
	@$(RUN) $(COMPILE_MACOS) -o $(BIN_MACOS) main.go
	@$(RUN) $(COMPILE_LINUX) -o $(BIN_LINUX) main.go

.PHONY: shell
shell:
	@$(RUN) sh

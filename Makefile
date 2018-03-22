IMAGE = opolis/build:dev
GOPATH = /go/src/github.com/opolis/build

RUN = docker run -it --rm \
	  -v $(HOME)/.aws:/root/.aws \
	  -v $(PWD):$(GOPATH) \
	  -v $(PWD)/.cache:/root/.cache/go-build \
	  -w $(GOPATH) \
	  $(IMAGE)

COMPILE = env GOOS=linux go build -ldflags="-s -w"

.PHONY: image
image:
	@docker build -t $(IMAGE) .

.PHONY: deps
deps:
	@$(RUN) dep ensure

.PHONY: build
build:
	@$(RUN) $(COMPILE) -o bin/builder builder/main.go
	@$(RUN) $(COMPILE) -o bin/listener listener/main.go
	@$(RUN) $(COMPILE) -o bin/notifier notifier/main.go

.PHONY: deploy
deploy:
	@$(RUN) serverless --stage dev deploy

.PHONY: update
update: build
	@$(RUN) serverless --stage dev deploy function -f builder
	@$(RUN) serverless --stage dev deploy function -f listener
	@$(RUN) serverless --stage dev deploy function -f notifier

.PHONY: shell
shell:
	@$(RUN) sh

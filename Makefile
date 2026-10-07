.PHONY: build run build-run encode decode

build:
	go build -o bin/url_shortener ./cmd/url_shortener

run:
	go run ./cmd/url_shortener $(ARGS)

build-run: build
	./bin/url_shortener $(ARGS)

encode: build
	./bin/url_shortener encode '$(URL)'

decode: build
	./bin/url_shortener decode '$(URL)'
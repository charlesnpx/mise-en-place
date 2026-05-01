.PHONY: build install test tidy clean

BIN := mise-en-place
CMD := ./cmd/mise-en-place

build:
	go build -o bin/$(BIN) $(CMD)

install:
	go install $(CMD)

test:
	go test ./...

tidy:
	go mod tidy

clean:
	rm -rf bin/ dist/

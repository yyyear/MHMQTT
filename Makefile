.PHONY: build run test clean

build:
	go build -o bin/mhmqtt main.go

run:
	go run main.go

test:
	go test ./...

clean:
	rm -rf bin/
	rm -rf data/
	rm -rf logs/

install:
	go mod download


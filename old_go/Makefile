.PHONY: build run test fmt clean

build:
	go build -o doi-app .

run:
	go run .

test:
	go test ./...

fmt:
	gofmt -w .
	go vet ./...

clean:
	rm -f doi-app

.PHONY: build run fmt clean

build:
	go build -o doi-app .

run:
	go run main.go

fmt:
	gofmt -w .
	go vet ./...

clean:
	rm -f doi-app

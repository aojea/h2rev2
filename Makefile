all: test

lint:
	golint ./...

vet:
	go vet -v ./...

test:
	go test -v ./... -count 1
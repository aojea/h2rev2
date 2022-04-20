all: test

test:
	go test -v -race . -count 1
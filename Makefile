build:
	go build -v --tags "fts5"


run: build
	./cowyo2

dev:
	rerun make run
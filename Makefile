all: install docker

install:
	@go mod download

build:
	@go build -o gptbot .

run: build
	@sh -c '[ -f .env ] || ( echo "Error! Missing .env file. See example.env for an example." && exit 1)'
	@./gptbot

test:
	@go test ./...

docker:
	@docker build -t gptbot:latest .

clean:
	@rm -f gptbot

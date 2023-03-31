all: build docker

build:
	@go build -o ./bin/gptbot .

run: build
	@sh -c '[ -f .env ] || ( echo "Error! Missing .env file. See example.env for an example." && exit 1)'
	@source .env && ./bin/gptbot

docker:
	@docker build -t gptbot:latest .

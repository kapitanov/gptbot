all: build test vet docker

build:
	go build -o ./bin/gptbot .

test:
	go test ./...

vet:
	go vet ./...

run: build _ensure_env_file
	source .env && ./bin/gptbot run -v

chat: build _ensure_env_file
	source .env && ./bin/gptbot chat

_ensure_env_file:
	@sh -c '[ -f .env ] || ( echo "Error! Missing .env file. See example.env for an example." && exit 1)'

docker:
	docker build -t gptbot:latest .

all: install docker

install:
	@npm install

run: install
	@sh -c '[ -f .env ] || ( echo "Error! Missing .env file. See example.env for an example." && exit 1)'
	@npm start

test:
	@npm test

docker:
	@docker build -t gptbot:latest .

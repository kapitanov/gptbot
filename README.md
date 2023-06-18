# gptbot

A telegram bot that uses GPT3 to transform text.

This bot is non-public, so you'll need to set up your own instance of this bot to use it.

## How to build and run

1. Clone git repository to any appropriate directory:

   ```shell
   cd /opt
   git clone https://github.com/kapitanov/gptbot.git
   cd gptbot
   ```

2. Create a `.env` file (see [configuration](#configuration) section below):

   ```env
   TELEGRAM_BOT_TOKEN=<telegram access token>
   TELEGRAM_BOT_ACCESS=<list of comma-separated telegram user ids and names>
   OPENAI_TOKEN=<place your openai token here>
   STORAGE_PATH=./var/data.yaml
   ```

   You'll need to:

    * get an access token for openai.com [here](https://platform.openai.com/account/api-keys)
    * get a bot api token for Telegram [here](http://t.me/BotFather)

3. Build and run docker container:

   ```shell
   docker-compose up -d --build
   ```

## Configuration

This bot is configured via env variables:

| Variable              | Default  | Description                                                      |
| --------------------- | -------- | ---------------------------------------------------------------- |
| `TELEGRAM_BOT_TOKEN`  | Required | Telegram bot API access token                                    |
| `TELEGRAM_BOT_ACCESS` | Required | List of allowed Telegram usernames (or userIDs), comma separated |
| `OPENAI_TOKEN`        | Required | OpenAI access token                                              |
| `STORAGE_PATH`        | Required | Path to message history file (YAML)                              |

## License

[MIT](LICENSE)

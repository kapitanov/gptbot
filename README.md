# gptbot

A telegram bot that uses GPT3 to transform text.

**Note: This repository has been converted from Go to JavaScript/Node.js while maintaining the same functionality.**

This bot is non-public, so you'll need to set up your own instance of this bot to use it.

## Requirements

- Node.js 18 or higher
- npm

## How to build and run

1. Clone git repository to any appropriate directory:

   ```shell
   cd /opt
   git clone https://github.com/kapitanov/gptbot.git
   cd gptbot
   ```

2. Install dependencies:

   ```shell
   npm install
   ```

3. Create a `.env` file (see [configuration](#configuration) section below):

   ```env
   TELEGRAM_BOT_TOKEN=<telegram access token>
   TELEGRAM_BOT_ACCESS=<list of comma-separated telegram user ids and names>
   OPENAI_TOKEN=<place your openai token here>
   STORAGE_PATH=./var/data.yaml
   ```

   You'll need to:

    * get an access token for openai.com [here](https://platform.openai.com/account/api-keys)
    * get a bot api token for Telegram [here](http://t.me/BotFather)

4. Run the bot:

   ```shell
   npm start
   ```

   Or with Docker:

   ```shell
   docker-compose up -d --build
   ```

## Development

Run tests:
```shell
npm test
```

Run in development mode with auto-restart:
```shell
npm run dev
```

## Configuration

This bot is configured via env variables:

| Variable              | Default  | Description                                                      |
| --------------------- | -------- | ---------------------------------------------------------------- |
| `TELEGRAM_BOT_TOKEN`  | Required | Telegram bot API access token                                    |
| `TELEGRAM_BOT_ACCESS` | Required | List of allowed Telegram usernames (or userIDs), comma separated |
| `OPENAI_TOKEN`        | Required | OpenAI access token                                              |
| `STORAGE_PATH`        | Required | Path to message history file (YAML)                              |

## Migration from Go

This repository has been fully converted from Go to JavaScript while preserving all functionality:

- **Language**: Go → JavaScript (Node.js 18+)
- **Dependencies**: Go modules → npm packages
- **Structure**: Maintained equivalent module structure in `src/` directory
- **Configuration**: Same environment variables and YAML config
- **Docker**: Updated for Node.js runtime
- **Functionality**: All original features preserved including GPT integration, Telegram bot, storage, and access control

## License

[MIT](LICENSE)

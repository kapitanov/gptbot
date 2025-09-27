import 'dotenv/config';
import winston from 'winston';
import { GPT } from './src/gpt/gpt.js';
import { Storage } from './src/storage/storage.js';
import { Telegram } from './src/telegram/telegram.js';

// Configure logger
const logger = winston.createLogger({
  level: 'debug',
  format: winston.format.combine(
    winston.format.timestamp({ format: 'YYYY-MM-DDTHH:mm:ss.SSSZ' }),
    winston.format.colorize(),
    winston.format.simple()
  ),
  transports: [
    new winston.transports.Console()
  ]
});

// Set global logger
global.logger = logger;

// AccessProvider checks access to telegram chats
class AccessProvider {
  constructor(accessString) {
    this.ids = new Set();
    this.usernames = new Set();

    if (!accessString) return;

    // Split by commas, semicolons, or spaces
    const entries = accessString.split(/[,;\s]+/).filter(Boolean);
    
    for (let entry of entries) {
      entry = entry.trim();
      
      // Try to parse as number (user ID)
      const id = parseInt(entry, 10);
      if (!isNaN(id)) {
        this.ids.add(id);
      } else {
        // Remove @ prefix if present and add as username
        const username = entry.replace(/^@/, '');
        this.usernames.add(username);
      }
    }
  }

  // CheckAccess checks access to telegram chat and returns true if access is granted
  checkAccess(id, username) {
    if (this.ids.has(id)) {
      return true;
    }

    if (this.usernames.has(username)) {
      return true;
    }

    return false;
  }
}

async function main() {
  try {
    // Initialize storage
    const storage = new Storage(process.env.STORAGE_PATH);
    await storage.initialize();

    // Initialize GPT
    const gpt = new GPT(process.env.OPENAI_TOKEN);
    await gpt.initialize();

    // Create access provider
    const accessProvider = new AccessProvider(process.env.TELEGRAM_BOT_ACCESS);

    // Initialize Telegram bot
    const telegram = new Telegram({
      token: process.env.TELEGRAM_BOT_TOKEN,
      accessChecker: accessProvider,
      gpt: gpt,
      storage: storage
    });

    // Handle graceful shutdown
    const shutdown = async () => {
      logger.info('Shutting down...');
      await telegram.close();
      logger.info('Good bye');
      process.exit(0);
    };

    process.on('SIGINT', shutdown);
    process.on('SIGTERM', shutdown);

    // Start the bot
    logger.info('Press Ctrl+C to exit');
    await telegram.run();
    
  } catch (error) {
    logger.error('Failed to start application:', error);
    process.exit(1);
  }
}

main();

export { AccessProvider };
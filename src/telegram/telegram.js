import TelegramBot from 'node-telegram-bot-api';
import { Participant } from '../gpt/gpt.js';
import { MessageSide } from '../storage/storage.js';
import texts from './texts/texts.js';
import { parse as mdParse } from './mdparser/mdparser.js';

const MAX_TEXT_LENGTH = 4095; // Telegram limit minus 1

class Telegram {
  constructor(options) {
    this.bot = new TelegramBot(options.token);
    this.storage = options.storage;
    this.gpt = options.gpt;
    this.accessChecker = options.accessChecker;
    this.botInfo = null;
  }

  async run() {
    // Get bot info
    this.botInfo = await this.bot.getMe();
    global.logger?.info(`Connected to Telegram - ID: ${this.botInfo.id}, Username: ${this.botInfo.username}`);

    this.setupHandlers();
    
    // Start polling
    this.bot.startPolling();
    
    return new Promise(() => {}); // Keep running forever
  }

  async close() {
    if (this.bot) {
      await this.bot.stopPolling();
      global.logger?.info('Telegram bot stopped');
    }
  }

  setupHandlers() {
    this.bot.onText(/\/start/, (msg) => this.onStartCommand(msg));
    this.bot.on('message', (msg) => {
      if (msg.text && !msg.text.startsWith('/')) {
        this.onText(msg);
      }
    });
    this.bot.on('photo', (msg) => this.onPhoto(msg));
    this.bot.on('video', (msg) => this.onVideo(msg));
    this.bot.on('audio', (msg) => this.onAudio(msg));
    this.bot.on('animation', (msg) => this.onAnimation(msg));
    this.bot.on('document', (msg) => this.onDocument(msg));
    this.bot.on('voice', (msg) => this.onVoice(msg));
  }

  async onStartCommand(msg) {
    if (!this.hasAccess(msg)) {
      return;
    }

    try {
      await this.bot.sendMessage(msg.chat.id, texts.Welcome);
    } catch (error) {
      global.logger?.error(`Failed to send welcome message to ${msg.from.username} (${msg.message_id}):`, error);
    }
  }

  async onText(msg) {
    await this.generate(msg, msg.text, '');
  }

  async onPhoto(msg) {
    await this.generate(msg, msg.caption || '', msg.caption || '');
  }

  async onVideo(msg) {
    await this.generate(msg, msg.caption || '', msg.caption || '');
  }

  async onAudio(msg) {
    await this.generate(msg, msg.caption || '', msg.caption || '');
  }

  async onAnimation(msg) {
    await this.generate(msg, msg.caption || '', msg.caption || '');
  }

  async onDocument(msg) {
    await this.generate(msg, msg.caption || '', msg.caption || '');
  }

  async onVoice(msg) {
    await this.generate(msg, msg.caption || '', msg.caption || '');
  }

  hasAccess(msg) {
    if (msg.from.id === this.botInfo?.id) {
      return true;
    }

    if (this.accessChecker.checkAccess(msg.from.id, msg.from.username)) {
      return true;
    }

    global.logger?.error(`Access denied for ${msg.from.username} (${msg.from.id})`);

    this.bot.sendMessage(msg.chat.id, texts.AccessDenied, {
      reply_to_message_id: msg.message_id
    }).catch(error => {
      global.logger?.error(`Failed to send access denied message to ${msg.from.username} (${msg.message_id}):`, error);
    });

    return false;
  }

  async generate(msg, text, altText) {
    if (!this.hasAccess(msg)) {
      return;
    }

    if (!text) {
      text = altText;
    }

    if (!text) {
      if (msg.media_group_id) {
        return; // Skip album messages without text
      }

      global.logger?.warn(`Empty text from ${msg.from.username} (${msg.message_id})`);

      try {
        await this.bot.sendMessage(msg.chat.id, texts.MissingText, {
          reply_to_message_id: msg.message_id
        });
      } catch (error) {
        global.logger?.error(`Failed to send missing text message to ${msg.from.username} (${msg.message_id}):`, error);
      }
      return;
    }

    try {
      await this.storage.tx(msg.from.id, async (chain) => {
        await this.generateE(msg, text, chain);
      });
    } catch (error) {
      global.logger?.error(`Failed to process message from ${msg.from.username} (${msg.message_id}):`, error, `Text: ${text}`);

      try {
        await this.bot.sendMessage(msg.chat.id, `${texts.Failure}\n${error.message}`, {
          reply_to_message_id: msg.message_id
        });
      } catch (replyError) {
        global.logger?.error(`Failed to send error message to ${msg.from.username} (${msg.message_id}):`, replyError);
      }
    }
  }

  async generateE(msg, request, chain) {
    const gptMessages = await this.generateGPTMessages(msg, request, chain);

    // Send "thinking" message
    const thinkingMsg = await this.bot.sendMessage(msg.chat.id, texts.Thinking, {
      reply_to_message_id: msg.message_id
    });

    // Send typing indicator
    await this.bot.sendChatAction(msg.chat.id, 'typing').catch(() => {
      // Ignore errors for typing indicator
    });

    // Generate response
    const response = await this.gpt.generate(gptMessages);

    // Send the actual reply
    const replyMsg = await this.reply(msg, thinkingMsg, response);

    // Store messages in conversation chain
    const replyToID = msg.reply_to_message ? msg.reply_to_message.message_id : null;
    await chain.store(msg.message_id, replyToID, MessageSide.USER, request);
    await chain.store(replyMsg.message_id, msg.message_id, MessageSide.BOT, response);

    global.logger?.info(`Generated reply for ${msg.from.username} (${msg.message_id}) - Request: ${request}, Response: ${response}`);
  }

  async reply(msg, thinkingMsg, response) {
    const [parsedResponse, entities] = mdParse(response);

    // Delete the "thinking" message
    await this.bot.deleteMessage(msg.chat.id, thinkingMsg.message_id).catch(() => {
      // Ignore errors when deleting thinking message
    });

    if (parsedResponse.length <= MAX_TEXT_LENGTH) {
      return await this.bot.sendMessage(msg.chat.id, parsedResponse, {
        reply_to_message_id: msg.message_id,
        entities: entities
      });
    } else {
      // Split long messages
      let remainingText = parsedResponse;
      let lastReply;

      while (remainingText.length > 0) {
        const text = remainingText.length <= MAX_TEXT_LENGTH 
          ? remainingText 
          : remainingText.substring(0, MAX_TEXT_LENGTH);
        
        remainingText = remainingText.length <= MAX_TEXT_LENGTH 
          ? '' 
          : remainingText.substring(MAX_TEXT_LENGTH);

        lastReply = await this.bot.sendMessage(msg.chat.id, text, {
          reply_to_message_id: msg.message_id
        });
      }

      return lastReply;
    }
  }

  async generateGPTMessages(msg, text, chain) {
    text = this.normalizeText(text);
    if (!text) {
      throw new Error('Text is empty');
    }

    const msgID = msg.reply_to_message ? msg.reply_to_message.message_id : 0;
    const storedMessages = chain.read(msgID);

    const gptMessages = storedMessages.map(storedMessage => ({
      participant: storedMessage.side === MessageSide.USER ? Participant.USER : Participant.BOT,
      text: storedMessage.text
    }));

    gptMessages.push({
      participant: Participant.USER,
      text: text
    });

    return gptMessages;
  }

  normalizeText(text) {
    text = text?.trim() || '';
    if (!text) {
      return '';
    }

    if (!text.endsWith('.')) {
      text = text + '.';
    }

    return text;
  }
}

export { Telegram };
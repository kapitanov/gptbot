import fs from 'fs/promises';
import yaml from 'yaml';

// MessageSide is a side of conversation
const MessageSide = {
  BOT: 'bot',
  USER: 'user'
};

// Storage stores conversation data
class Storage {
  constructor(filename) {
    this.filename = filename || './var/data.yaml';
    this.mutex = Promise.resolve(); // Simple mutex using promise chain
  }

  async initialize() {
    // Test if we can read/create the file
    await this.do(async () => {
      // Just a test operation
    });
  }

  // TX runs a function within a transaction
  async tx(userID, fn) {
    return this.do(async (root, save) => {
      let conversation = root.conversations[userID];
      if (!conversation) {
        conversation = { messages: {} };
        root.conversations[userID] = conversation;
      }

      if (!conversation.messages) {
        conversation.messages = {};
      }

      const messageChain = new MessageChain(conversation, save);
      return await fn(messageChain);
    });
  }

  async do(fn) {
    // Simple mutex implementation using promise chaining
    return this.mutex = this.mutex.then(async () => {
      const root = await this.load();
      
      const save = async () => {
        await this.store(root);
      };

      return await fn(root, save);
    });
  }

  async load() {
    try {
      const data = await fs.readFile(this.filename, 'utf8');
      const root = yaml.parse(data) || {};
      
      if (!root.conversations) {
        root.conversations = {};
      }

      return root;
    } catch (error) {
      if (error.code === 'ENOENT') {
        // File doesn't exist, create it
        await fs.writeFile(this.filename, '', 'utf8');
        return { conversations: {} };
      }
      throw error;
    }
  }

  async store(root) {
    const data = yaml.stringify(root);
    await fs.writeFile(this.filename, data, 'utf8');
  }
}

// MessageChain is a single conversation
class MessageChain {
  constructor(conversation, save) {
    this.conversation = conversation;
    this.save = save;
  }

  // Store writes new message into the conversation
  async store(msgID, replyToID, side, text) {
    const msg = {
      side: side,
      text: text
    };

    if (replyToID !== null && replyToID !== undefined) {
      msg.reply_to = replyToID;
    }

    this.conversation.messages[msgID] = msg;
    await this.save();
  }

  // Read reads all messages from the conversation
  read(messageID) {
    const messages = [];
    let currentID = messageID;

    while (true) {
      const msg = this.conversation.messages[currentID];
      if (!msg) {
        break;
      }

      messages.push({
        side: msg.side,
        text: msg.text
      });

      if (!msg.reply_to) {
        break;
      }

      currentID = msg.reply_to;
    }

    // Reverse to get chronological order
    return messages.reverse();
  }
}

export { Storage, MessageChain, MessageSide };
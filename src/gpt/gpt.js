import OpenAI from 'openai';
import { GPTConfig } from './config.js';

// MaxConversationDepth limits conversation depth
const MAX_CONVERSATION_DEPTH = 5;

// Participant is the side of conversation
const Participant = {
  BOT: 'bot',
  USER: 'user'
};

// GPT is a GPT text transformer
class GPT {
  constructor(token) {
    this.client = new OpenAI({
      apiKey: token
    });
  }

  async initialize() {
    try {
      // Test the connection
      await this.client.models.list();
    } catch (error) {
      throw new Error(`Failed to initialize OpenAI client: ${error.message}`);
    }
  }

  // Generate generates a new message from the input stream
  async generate(messages) {
    const request = await this.createChatCompletionRequest(messages);

    // Log request messages
    for (const message of request.messages) {
      global.logger?.debug(`GPT request - Role: ${message.role}, Content: ${message.content}`);
    }

    try {
      const response = await this.client.chat.completions.create(request);

      // Log response
      for (const choice of response.choices) {
        global.logger?.debug(`GPT response - Role: ${choice.message.role}, Content: ${choice.message.content}, Finish: ${choice.finish_reason}`);
      }

      global.logger?.debug(`GPT stats - Model: ${response.model}, Total tokens: ${response.usage?.total_tokens}, Prompt tokens: ${response.usage?.prompt_tokens}, Response tokens: ${response.usage?.completion_tokens}`);

      return response.choices[0].message.content;
    } catch (error) {
      throw new Error(`OpenAI API error: ${error.message}`);
    }
  }

  async createChatCompletionRequest(messages) {
    const config = await GPTConfig.load();
    
    const request = {
      model: config.model,
      temperature: config.temperature,
      messages: [
        {
          role: 'system',
          content: config.prompt
        }
      ]
    };

    // Limit conversation depth
    let limitedMessages = messages;
    if (messages.length > MAX_CONVERSATION_DEPTH) {
      limitedMessages = messages.slice(-MAX_CONVERSATION_DEPTH);
    }

    // Convert messages to OpenAI format
    for (const message of limitedMessages) {
      const role = message.participant === Participant.BOT ? 'assistant' : 'user';
      
      request.messages.push({
        role: role,
        content: message.text
      });
    }

    return request;
  }
}

export { GPT, Participant };
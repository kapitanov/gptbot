import fs from 'fs/promises';
import yaml from 'yaml';

class GPTConfig {
  constructor() {
    this.model = 'gpt-3.5-turbo';
    this.temperature = 0.9;
    this.prompt = `    Как бы малообразованный человек в состоянии алкогольного опьянения, ` +
                 ` разговаривающий матом и эмоционально, пересказал бы следующую новость?` +
                 `Комментарий должен быть эмоциональным, просторечным и матерным.`;
  }

  static async load() {
    const defaultSourcePath = './conf/gpt.yaml';
    const sourcePath = process.env.CONFIG_PATH || defaultSourcePath;

    try {
      const data = await fs.readFile(sourcePath, 'utf8');
      const config = yaml.parse(data);
      
      const gptConfig = new GPTConfig();
      if (config.model) gptConfig.model = config.model;
      if (config.temperature !== undefined) gptConfig.temperature = config.temperature;
      if (config.prompt) gptConfig.prompt = config.prompt;
      
      return gptConfig;
    } catch (error) {
      global.logger?.error(`Unable to load gpt config from ${sourcePath}:`, error.message);
      return new GPTConfig(); // Return default config
    }
  }
}

export { GPTConfig };
import { test } from 'node:test';
import assert from 'node:assert';
import { parse } from '../src/telegram/mdparser/mdparser.js';

test('mdparser functionality', async (t) => {
  const testCases = [
    {
      name: 'PlainText',
      input: 'Hello world!',
      expectedText: 'Hello world!',
      expectedEntities: []
    },
    {
      name: 'Bold',
      input: 'Hello **world!**',
      expectedText: 'Hello world!',
      expectedEntities: [
        { type: 'bold', offset: 6, length: 6 }
      ]
    },
    {
      name: 'Italic',
      input: 'Hello *world!*',
      expectedText: 'Hello world!',
      expectedEntities: [
        { type: 'italic', offset: 6, length: 6 }
      ]
    },
    {
      name: 'ItalicAlt',
      input: 'Hello _world!_',
      expectedText: 'Hello world!',
      expectedEntities: [
        { type: 'italic', offset: 6, length: 6 }
      ]
    },
    {
      name: 'Strikethrough',
      input: 'Hello ~~world!~~',
      expectedText: 'Hello world!',
      expectedEntities: [
        { type: 'strikethrough', offset: 6, length: 6 }
      ]
    },
    {
      name: 'Code',
      input: 'Hello `world!`',
      expectedText: 'Hello world!',
      expectedEntities: [
        { type: 'code', offset: 6, length: 6 }
      ]
    },
    {
      name: 'Hyperlink',
      input: 'Hello [world](https://example.com)!',
      expectedText: 'Hello world!',
      expectedEntities: [
        { type: 'text_link', offset: 6, length: 5, url: 'https://example.com' }
      ]
    },
    {
      name: 'CodeBlock',
      input: 'Hello\n\n```\nSource Code\n```',
      expectedText: 'Hello\nSource Code',
      expectedEntities: [
        { type: 'pre', offset: 6, length: 11 }
      ]
    },
    {
      name: 'CodeBlockWithLanguage',
      input: 'Hello\n\n```bash\nSource Code\n```',
      expectedText: 'Hello\nSource Code',
      expectedEntities: [
        { type: 'pre', offset: 6, length: 11, language: 'bash' }
      ]
    },
    {
      name: 'Heading1',
      input: '# Hello World!',
      expectedText: 'Hello World!',
      expectedEntities: [
        { type: 'underline', offset: 0, length: 12 },
        { type: 'bold', offset: 0, length: 12 }
      ]
    },
    {
      name: 'Heading2',
      input: '## Hello World!',
      expectedText: 'Hello World!',
      expectedEntities: [
        { type: 'underline', offset: 0, length: 12 }
      ]
    },
    {
      name: 'BulletList1',
      input: '- Item 1\n- Item 2\n- Item 3',
      expectedText: '• Item 1\n• Item 2\n• Item 3',
      expectedEntities: []
    },
    {
      name: 'BulletList2',
      input: '* Item 1\n* Item 2\n* Item 3',
      expectedText: '• Item 1\n• Item 2\n• Item 3',
      expectedEntities: []
    },
    {
      name: 'OrderedList',
      input: '1. Item 1\n1. Item 2\n1. Item 3',
      expectedText: '• 1. Item 1\n• 2. Item 2\n• 3. Item 3',
      expectedEntities: []
    },
    {
      name: 'BlockQuote',
      input: 'Hello\n> Quote',
      expectedText: 'HelloQuote',
      expectedEntities: [
        { type: 'blockquote', offset: 5, length: 5 }
      ]
    }
  ];

  for (const tc of testCases) {
    await t.test(tc.name, () => {
      const [actualText, actualEntities] = parse(tc.input);
      
      console.log(`Test: ${tc.name}`);
      console.log(`Input: ${JSON.stringify(tc.input)}`);
      console.log(`Expected: ${JSON.stringify(tc.expectedText)}`);
      console.log(`Actual: ${JSON.stringify(actualText)}`);
      console.log(`Expected entities: ${JSON.stringify(tc.expectedEntities)}`);
      console.log(`Actual entities: ${JSON.stringify(actualEntities)}`);
      console.log('---');

      assert.strictEqual(actualText, tc.expectedText, `Text mismatch for ${tc.name}`);
      
      // Compare entities with some flexibility for ordering
      assert.strictEqual(actualEntities.length, tc.expectedEntities.length, 
        `Entity count mismatch for ${tc.name}`);
      
      for (let i = 0; i < tc.expectedEntities.length; i++) {
        const expected = tc.expectedEntities[i];
        const actual = actualEntities[i];
        
        assert.strictEqual(actual.type, expected.type, `Entity type mismatch at ${i} for ${tc.name}`);
        assert.strictEqual(actual.offset, expected.offset, `Entity offset mismatch at ${i} for ${tc.name}`);
        assert.strictEqual(actual.length, expected.length, `Entity length mismatch at ${i} for ${tc.name}`);
        
        if (expected.url) {
          assert.strictEqual(actual.url, expected.url, `Entity URL mismatch at ${i} for ${tc.name}`);
        }
        
        if (expected.language) {
          assert.strictEqual(actual.language, expected.language, `Entity language mismatch at ${i} for ${tc.name}`);
        }
      }
    });
  }
});
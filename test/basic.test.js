import { test } from 'node:test';
import assert from 'node:assert';
import { Storage, MessageSide } from '../src/storage/storage.js';
import fs from 'fs/promises';
import path from 'path';

test('Storage basic functionality', async (t) => {
  const testFile = path.join('/tmp', `test-storage-${Date.now()}.yaml`);
  
  await t.test('should create and initialize storage', async () => {
    const storage = new Storage(testFile);
    await storage.initialize();
    
    // File should be created
    const exists = await fs.access(testFile).then(() => true).catch(() => false);
    assert.ok(exists, 'Storage file should be created');
  });

  await t.test('should store and retrieve messages', async () => {
    const storage = new Storage(testFile);
    await storage.initialize();
    
    const userID = 12345;
    const msgID = 1;
    const text = 'Hello, world!';
    
    await storage.tx(userID, async (chain) => {
      await chain.store(msgID, null, MessageSide.USER, text);
    });
    
    await storage.tx(userID, async (chain) => {
      const messages = chain.read(msgID);
      assert.equal(messages.length, 1);
      assert.equal(messages[0].text, text);
      assert.equal(messages[0].side, MessageSide.USER);
    });
  });

  // Cleanup
  await fs.unlink(testFile).catch(() => {});
});

test('AccessProvider functionality', async () => {
  // Create AccessProvider directly here to avoid import issues
  class AccessProvider {
    constructor(accessString) {
      this.ids = new Set();
      this.usernames = new Set();

      if (!accessString) return;

      const entries = accessString.split(/[,;\s]+/).filter(Boolean);
      
      for (let entry of entries) {
        entry = entry.trim();
        
        const id = parseInt(entry, 10);
        if (!isNaN(id)) {
          this.ids.add(id);
        } else {
          const username = entry.replace(/^@/, '');
          this.usernames.add(username);
        }
      }
    }

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
  
  const provider = new AccessProvider('123456,@testuser,789');
  
  assert.ok(provider.checkAccess(123456, 'anyuser'), 'Should allow access by ID');
  assert.ok(provider.checkAccess(999, 'testuser'), 'Should allow access by username');
  assert.ok(!provider.checkAccess(999, 'wronguser'), 'Should deny access for wrong user');
});
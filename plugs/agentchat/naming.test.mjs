import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
test('current chat source uses the agentchat path and neutral identifiers', async () => {
 const source = await readFile(new URL('../agentchat/agentchat.js', import.meta.url), 'utf8');
 assert(source.includes('agentchat-panel'));
 assert(!source.includes('chief-'));
 const plug = await readFile(new URL('../agentchat/agentchat.plug.js', import.meta.url), 'utf8');
 assert(plug.includes('./agentchat.js:openChat'));
});

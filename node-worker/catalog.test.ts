import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { modules } from './nodes/index.js';

test('every catalog action has exactly one registered executor and request policy', () => {
  const catalog: {type:string;version:number;kind:string;httpPolicy?:unknown}[] = JSON.parse(readFileSync('contracts/product/catalog.json','utf8'));
  const actions = catalog.filter(d => d.kind === 'action');
  const keys = actions.map(d => `${d.type}@${d.version}`);
  assert.equal(new Set(keys).size, keys.length, 'duplicate action descriptor');
  assert.deepEqual([...modules.keys()].sort(), keys.sort());
  for (const action of actions) {
    if (action.type !== 'mock.echo') assert.ok(action.httpPolicy, `${action.type} needs a declared request policy`);
  }
});

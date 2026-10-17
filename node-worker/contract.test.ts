import { readFileSync } from 'node:fs';
import test from 'node:test';
import assert from 'node:assert/strict';
import { validateRequest } from './contract.js';

const fixtures = JSON.parse(readFileSync('contracts/spike/fixtures.json', 'utf8'));
for (const row of fixtures.cases) {
  test(`shared contract: ${row.name}`, () => {
    const call = () => validateRequest({ ...fixtures.base, ...row.patch });
    if (row.valid) assert.doesNotThrow(call); else assert.throws(call);
  });
}
test('reject missing, oversized, and deeply nested payloads', () => {
  assert.throws(() => validateRequest({}));
  assert.throws(() => validateRequest({ ...fixtures.base, value: { text: 'x'.repeat(8193) } }));
  let value: object = {};
  for (let i = 0; i < 18; i++) value = { nested: value };
  assert.throws(() => validateRequest({ ...fixtures.base, value }));
});

import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { ApplicationFailure, CancelledFailure } from '@temporalio/common';
import { runNode, providerEndpoint, type Environment } from './activity.js';

const base = JSON.parse(readFileSync('contracts/spike/fixtures.json', 'utf8')).base;
const environment: Environment = { version: 'v1', providerUrl: 'http://127.0.0.1:8181', attempt: 1, signal: new AbortController().signal };

test('successful result preserves the cross-language payload', async () => {
  const result = await runNode(base, { ...environment, request: async () => new Response(JSON.stringify({ id: 'action-1', value: base.value })) });
  assert.deepEqual(result.value, base.value);
  assert.equal(result.workerVersion, 'v1');
});
test('reject incompatible versions before sending a request', async () => {
  let called = false;
  await assert.rejects(runNode({ ...base, nodeVersion: 'v2' }, { ...environment, request: async () => { called = true; return new Response(); } }), (e: unknown) => e instanceof ApplicationFailure && e.type === 'VersionMismatch' && e.nonRetryable);
  assert.equal(called, false);
});
test('classify and sanitize retryable errors', async () => {
  for (const response of [new Response('SENTINEL_SECRET', { status: 503 }), new Response('SENTINEL_SECRET', { status: 429 })]) {
    await assert.rejects(runNode(base, { ...environment, request: async () => response }), (e: unknown) => e instanceof ApplicationFailure && e.type === 'ProviderTransient' && !e.message.includes('SENTINEL_SECRET'));
  }
  await assert.rejects(runNode(base, { ...environment, request: async () => { throw new Error('SENTINEL_SECRET'); } }), (e: unknown) => e instanceof ApplicationFailure && e.type === 'ProviderNetwork' && !e.message.includes('SENTINEL_SECRET'));
});
test('reject oversized and invalid provider data without capturing it', async () => {
  for (const text of ['SENTINEL_SECRET', 'x'.repeat(8193), '{"id":9,"value":{}}']) {
    await assert.rejects(runNode(base, { ...environment, request: async () => new Response(text) }), (e: unknown) => e instanceof ApplicationFailure && e.type === 'InvalidProviderResponse' && e.nonRetryable && !e.message.includes('SENTINEL_SECRET'));
  }
});
test('cancellation remains cancellation', async () => {
  const controller = new AbortController(); controller.abort();
  await assert.rejects(runNode(base, { ...environment, signal: controller.signal, request: async () => { throw new Error('aborted'); } }), CancelledFailure);
});
test('development HTTP destination is fixed to loopback', () => {
  for (const url of ['https://example.com', 'http://169.254.169.254', 'http://user:pass@127.0.0.1', 'http://127.0.0.1/redirect']) assert.throws(() => providerEndpoint(url));
  assert.equal(providerEndpoint('http://127.0.0.1:8181').pathname, '/actions');
});

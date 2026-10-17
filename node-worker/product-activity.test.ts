import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { runInternalNode, serviceOrigin } from './product-activity.js';
const catalog = JSON.parse(readFileSync('contracts/product/catalog.json', 'utf8'));
const fixture = JSON.parse(readFileSync('contracts/product/activity.fixtures.json', 'utf8'))[0].value;
const task = { ...fixture, nodeType: 'http.post', implementationDigest: catalog[0].implementationDigest };
test('HTTP module delegates scoped request and persists completion without credentials', async () => {
 const routes: string[] = [];
 const request: typeof fetch = async (url, options) => {
  const path = new URL(String(url)).pathname; routes.push(path);
  const data = JSON.parse(String(options?.body)); assert.deepEqual(data.envelope, task);
  if (path.endsWith('/input')) return Response.json({ input: { title: 'fixture' }, config: { url: 'https://example.com/tasks', credentialRef: 'credential_test' } });
  if (path.endsWith('/request')) { assert.deepEqual(data.request, { url: 'https://example.com/tasks', method: 'POST', body: { title: 'fixture' } }); return Response.json({ status: 201, body: { id: 'fake' } }); }
  assert.deepEqual(data.output, { status: 201, body: { id: 'fake' } });
  return Response.json({ outputRef: 'payload_result', attempt: 2 });
 };
 assert.deepEqual(await runInternalNode(task, { serviceUrl: 'http://127.0.0.1:8788', token: 'fake-worker-token', attempt: 2, signal: new AbortController().signal, request }), { outputRef: 'payload_result', attempt: 2 });
 assert.deepEqual(routes, ['/internal/node/input', '/internal/node/request', '/internal/node/complete']);
});
test('provider and transport errors expose only stable codes', async () => {
 for (const request of [async () => { throw new Error('sentinel-secret'); }, async () => Response.json({ code: 'sentinel-secret', retryable: false }, { status: 500 })]) {
  await assert.rejects(runInternalNode(task, { serviceUrl: 'http://127.0.0.1:8788', token: 'fake', attempt: 1, signal: new AbortController().signal, request }), error => {
   assert.ok(error instanceof Error); assert.equal(error.message, 'Node execution failed'); assert.ok(!JSON.stringify(error).includes('sentinel-secret')); return true;
  });
 }
});
test('internal service requires TLS or literal loopback without URL credentials', () => {
 for (const url of ['http://example.com', 'http://localhost:8788', 'https://u:p@example.com', 'https://example.com/path']) assert.throws(() => serviceOrigin(url));
});

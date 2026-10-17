import { readFileSync } from 'node:fs';
import { Context } from '@temporalio/activity';
import { NativeConnection, Worker } from '@temporalio/worker';
import { runInternalNode, serviceOrigin } from './product-activity.js';
const serviceUrl = process.env.RUNNELIX_NODE_SERVICE_URL ?? 'http://127.0.0.1:8788';
const token = process.env.RUNNELIX_WORKER_TOKEN ?? '';
if (token.length < 32) throw new Error('RUNNELIX_WORKER_TOKEN is required');
serviceOrigin(serviceUrl);
const catalog = JSON.parse(readFileSync(new URL('../../contracts/product/catalog.json', import.meta.url), 'utf8')) as { implementationDigest: string }[];
const digest = catalog[0]?.implementationDigest;
if (!digest || catalog.some(n => n.implementationDigest !== digest)) throw new Error('Invalid worker bundle identity');
const connection = await NativeConnection.connect({ address: process.env.TEMPORAL_ADDRESS ?? '127.0.0.1:7233' });
try {
  const worker = await Worker.create({ connection, taskQueue: `runnelix-nodes-${digest.slice(7)}`, maxConcurrentActivityTaskExecutions: 4,
    maxHeartbeatThrottleInterval: '500ms', defaultHeartbeatThrottleInterval: '200ms',
    activities: { ExecuteInternalNodeV1: async (input: unknown) => {
      const context = Context.current(); const timer = setInterval(() => context.heartbeat(), 200);
      try { return await runInternalNode(input, { serviceUrl, token, attempt: context.info.attempt, signal: context.cancellationSignal }); }
      finally { clearInterval(timer); }
    } },
  });
  await worker.run();
} finally { await connection.close(); }

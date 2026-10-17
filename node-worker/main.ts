import { Context } from '@temporalio/activity';
import { NativeConnection, Worker } from '@temporalio/worker';
import { providerEndpoint, runNode } from './activity.js';

const version = process.env.RUNNELIX_NODE_VERSION ?? 'v1';
if (version !== 'v1' && version !== 'v2') throw new Error('Unsupported node worker version');
const providerUrl = process.env.RUNNELIX_PROVIDER_URL ?? 'http://127.0.0.1:8181';
providerEndpoint(providerUrl);
const connection = await NativeConnection.connect({ address: process.env.TEMPORAL_ADDRESS ?? '127.0.0.1:7233' });
try {
  const worker = await Worker.create({
    connection,
    taskQueue: `runnelix-spike-nodes-${version}`,
    activities: {
      RunNode: async (input: unknown) => {
        const context = Context.current();
        const timer = setInterval(() => context.heartbeat(), 100);
        try {
          return await runNode(input, { version, providerUrl, attempt: context.info.attempt, signal: context.cancellationSignal });
        } finally { clearInterval(timer); }
      },
    },
    maxConcurrentActivityTaskExecutions: 4,
    maxHeartbeatThrottleInterval: '200ms',
    defaultHeartbeatThrottleInterval: '100ms',
  });
  await worker.run();
} finally { await connection.close(); }

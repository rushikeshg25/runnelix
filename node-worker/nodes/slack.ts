import type { Executor } from './index.js';
import { NodeFailure } from '../product-errors.js';

export const postMessage: Executor = async (input, config, signal, capabilities) => {
  signal.throwIfAborted();
  if (!capabilities) throw new NodeFailure('InvalidInput', 'Request capability unavailable');
  const { channel } = config as { channel: string };
  const { text } = input as { text: string };
  const result = await capabilities.request({
    url: 'https://slack.com/api/chat.postMessage', method: 'POST', body: { channel, text },
  }) as { status: number; body: Record<string, unknown> };
  if (result.status !== 200 || !result.body || typeof result.body !== 'object') throw new NodeFailure('InvalidOutput', 'Invalid message response');
  // Slack reports API rejection inside an HTTP 200 response. Never expose its raw error.
  if (result.body.ok === false) throw new NodeFailure('ProviderRejected', 'Message rejected');
  if (result.body.ok !== true) throw new NodeFailure('InvalidOutput', 'Invalid message response');
  return { channel: result.body.channel, ts: result.body.ts };
};

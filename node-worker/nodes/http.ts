import type { Capabilities, Executor } from './index.js';
// Credentials remain in the Go data plane; modules only receive a scoped request capability.
async function request(method: 'GET' | 'POST', input: unknown, config: unknown, signal: AbortSignal, capabilities?: Capabilities): Promise<unknown> {
  signal.throwIfAborted();
  if (!capabilities) throw new Error('Request capability unavailable');
  const { url } = config as { url: string };
  return capabilities.request({ url, method, ...(method === 'POST' ? { body: input } : {}) });
}
export const get: Executor = (input, config, signal, capabilities) => request('GET', input, config, signal, capabilities);
export const post: Executor = (input, config, signal, capabilities) => request('POST', input, config, signal, capabilities);

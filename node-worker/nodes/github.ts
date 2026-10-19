import type { Executor } from './index.js';
import { NodeFailure } from '../product-errors.js';

export const createIssue: Executor = async (input, config, signal, capabilities) => {
  signal.throwIfAborted();
  if (!capabilities) throw new NodeFailure('InvalidInput', 'Request capability unavailable');
  const { owner, repository } = config as { owner: string; repository: string };
  const result = await capabilities.request({
    url: `https://api.github.com/repos/${encodeURIComponent(owner)}/${encodeURIComponent(repository)}/issues`,
    method: 'POST', body: input,
  }) as { status: number; body: Record<string, unknown> };
  if (result.status !== 201 || !result.body || typeof result.body !== 'object') throw new NodeFailure('InvalidOutput', 'Invalid issue response');
  return { id: result.body.id, number: result.body.number, url: result.body.html_url };
};

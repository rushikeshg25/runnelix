import { ApplicationFailure, CancelledFailure } from '@temporalio/common';
import { executeNode, NodeFailure, parseEnvelope } from './product-host.js';

export interface Environment {
  serviceUrl: string; token: string; attempt: number; signal: AbortSignal; request?: typeof fetch;
}
const safeCodes = new Set(['RateLimited', 'InvalidInput', 'VersionMismatch', 'PayloadUnavailable', 'InvalidOutput', 'ProviderTransient', 'ProviderNetwork', 'ProviderRejected', 'EgressDenied', 'CredentialUnavailable', 'Cancelled', 'StorageUnavailable']);
class ServiceFailure extends Error {
  constructor(readonly code: string, readonly retryable: boolean) { super(code); }
}
export function serviceOrigin(raw: string): URL {
  const url = new URL(raw);
  if (url.username || url.password || url.pathname !== '/' || url.search || url.hash || (url.protocol !== 'https:' && !(url.protocol === 'http:' && ['127.0.0.1', '[::1]'].includes(url.hostname)))) throw new Error('Invalid internal service origin');
  return url;
}
async function readBody(response: Response): Promise<unknown> {
  if (!response.body) throw new ServiceFailure('StorageUnavailable', true);
  const reader = response.body.getReader(); const chunks: Uint8Array[] = []; let size = 0;
  try {
    while (true) {
      const { done, value } = await reader.read(); if (done) break;
      size += value.length;
      if (size > 24000) { await reader.cancel(); throw new ServiceFailure('InvalidOutput', false); }
      chunks.push(value);
    }
    return JSON.parse(Buffer.concat(chunks).toString('utf8')) as unknown;
  } finally { reader.releaseLock(); }
}
export async function runInternalNode(raw: unknown, env: Environment): Promise<unknown> {
  let envelope;
  try { envelope = parseEnvelope(raw); } catch { throw ApplicationFailure.nonRetryable('Invalid invocation', 'InvalidInput'); }
  const origin = serviceOrigin(env.serviceUrl);
  const call = async (route: string, fields: object = {}): Promise<unknown> => {
    try {
      const response = await (env.request ?? fetch)(new URL(`/internal/node/${route}`, origin), {
        method: 'POST', redirect: 'error', headers: { authorization: `Bearer ${env.token}`, 'content-type': 'application/json' },
        signal: AbortSignal.any([env.signal, AbortSignal.timeout(8000)]), body: JSON.stringify({ envelope, ...fields }),
      });
      const value = await readBody(response);
      if (!response.ok) {
        const failure = value as { code?: string; retryable?: boolean };
        throw new ServiceFailure(safeCodes.has(failure.code ?? '') ? failure.code! : 'StorageUnavailable', failure.retryable === true);
      }
      return value;
    } catch (error) {
      if (error instanceof ServiceFailure) throw error;
      throw new ServiceFailure('StorageUnavailable', true);
    }
  };
  try {
    const data = await call('input') as { input: unknown; config: unknown };
    const output = await executeNode(envelope, {
      signal: env.signal,
      resolve: async (workspace, run, ref) => {
        if (workspace !== envelope.workspaceId || run !== envelope.runId) throw new Error('Scope rejected');
        if (ref === envelope.inputRef) return data.input;
        if (ref === envelope.configRef) return data.config;
        throw new Error('Reference rejected');
      },
      capabilities: { request: request => call('request', { request }) },
    });
    return await call('complete', { attempt: env.attempt, status: 'succeeded', output });
  } catch (error) {
    if (env.signal.aborted) throw new CancelledFailure('Node cancelled');
    const code = error instanceof NodeFailure || error instanceof ServiceFailure ? error.code : 'InvalidOutput';
    const retryable = error instanceof ServiceFailure && error.retryable;
    // Best-effort failure evidence; a failed completion never becomes a successful activity.
    try { await call('complete', { attempt: env.attempt, status: 'failed', errorCode: code === 'StorageUnavailable' ? 'PayloadUnavailable' : code }); } catch { /* Original safe failure remains authoritative. */ }
    throw ApplicationFailure.create({ message: 'Node execution failed', type: code, nonRetryable: !retryable });
  }
}

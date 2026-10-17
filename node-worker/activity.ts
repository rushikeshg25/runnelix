import { ApplicationFailure, CancelledFailure } from '@temporalio/common';
import { validateRequest, type Result } from './contract.js';

export interface Environment {
  version: 'v1' | 'v2';
  providerUrl: string;
  attempt: number;
  signal: AbortSignal;
  request?: typeof fetch;
}

export function providerEndpoint(base: string): URL {
  const url = new URL(base);
  if (url.protocol !== 'http:' || !['127.0.0.1', '[::1]'].includes(url.hostname) || url.username || url.password || url.pathname !== '/' || url.search || url.hash) {
    throw new Error('The spike only supports a loopback mock-provider origin');
  }
  return new URL('/actions', url);
}

async function boundedBody(response: Response): Promise<unknown> {
  if (!response.body) throw new Error('empty response');
  const reader = response.body.getReader();
  let size = 0;
  const chunks: Uint8Array[] = [];
  try {
    while (true) {
      const chunk = await reader.read();
      if (chunk.done) break;
      size += chunk.value.length;
      if (size > 8192) { await reader.cancel(); throw new Error('response exceeds limit'); }
      chunks.push(chunk.value);
    }
  } finally { reader.releaseLock(); }
  return JSON.parse(Buffer.concat(chunks).toString('utf8'));
}

export async function runNode(input: unknown, environment: Environment): Promise<Result> {
  let invocation;
  try { invocation = validateRequest(input); }
  catch { throw ApplicationFailure.nonRetryable('Invalid invocation', 'InvalidInput'); }
  if (invocation.nodeVersion !== environment.version) {
    throw ApplicationFailure.nonRetryable('Worker does not support the pinned version', 'VersionMismatch');
  }
  const endpoint = providerEndpoint(environment.providerUrl);
  const signal = AbortSignal.any([environment.signal, AbortSignal.timeout(2500)]);
  try {
    const response = await (environment.request ?? fetch)(endpoint, {
      method: 'POST', redirect: 'error', signal,
      headers: { 'content-type': 'application/json' }, body: JSON.stringify(invocation),
    });
    if (response.status === 429 || response.status >= 500) {
      await response.body?.cancel();
      throw ApplicationFailure.retryable('Provider temporarily unavailable', 'ProviderTransient');
    }
    if (!response.ok) {
      await response.body?.cancel();
      throw ApplicationFailure.nonRetryable('Provider rejected request', 'ProviderRejected');
    }
    let action;
    try {
      const body = await boundedBody(response);
      if (typeof body !== 'object' || body === null || !('id' in body) || typeof body.id !== 'string' || !body.id || !('value' in body)) throw new Error('invalid shape');
      const validated = validateRequest({ ...invocation, value: body.value });
      action = { id: body.id, value: validated.value };
    } catch {
      if (environment.signal.aborted) throw new CancelledFailure('Activity cancelled');
      throw ApplicationFailure.nonRetryable('Invalid provider response', 'InvalidProviderResponse');
    }
    return { status: 'succeeded', actionId: action.id, workerVersion: environment.version, attempts: environment.attempt, value: action.value };
  } catch (error) {
    if (environment.signal.aborted) throw new CancelledFailure('Activity cancelled');
    if (error instanceof ApplicationFailure) throw error;
    // Do not place raw network errors, response bodies, or request data in history.
    throw ApplicationFailure.retryable('Provider request did not complete', 'ProviderNetwork');
  }
}

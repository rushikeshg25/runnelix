export type Json = null | boolean | string | number | Json[] | { [key: string]: Json };
export interface Request {
  contractVersion: 1;
  runId: string;
  nodeVersion: 'v1' | 'v2';
  mode: 'safe' | 'unsafe';
  fault: 'none' | 'transient-once' | 'disconnect' | 'hold';
  waitMs: number;
  value: { [key: string]: Json };
}
export interface Result {
  status: 'succeeded' | 'needs_attention';
  actionId: string;
  workerVersion: string;
  attempts: number;
  value: { [key: string]: Json };
}
function object(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v);
}
function json(v: unknown, depth = 0): boolean {
  if (depth > 16) return false;
  if (v === null || typeof v === 'string' || typeof v === 'boolean') return true;
  if (typeof v === 'number') return Number.isFinite(v) && Math.abs(v) <= Number.MAX_SAFE_INTEGER;
  if (Array.isArray(v)) return v.every(x => json(x, depth + 1));
  return object(v) && Object.values(v).every(x => json(x, depth + 1));
}
export function validateRequest(input: unknown): Request {
  const keys = ['contractVersion', 'runId', 'nodeVersion', 'mode', 'fault', 'waitMs', 'value'];
  if (!object(input) || Object.keys(input).length !== keys.length || !keys.every(k => Object.hasOwn(input, k)) ||
    input.contractVersion !== 1 || typeof input.runId !== 'string' || !/^[a-zA-Z0-9_-]{1,80}$/.test(input.runId) ||
    (input.nodeVersion !== 'v1' && input.nodeVersion !== 'v2') || (input.mode !== 'safe' && input.mode !== 'unsafe') ||
    (input.fault !== 'none' && input.fault !== 'transient-once' && input.fault !== 'disconnect' && input.fault !== 'hold') ||
    typeof input.waitMs !== 'number' || !Number.isInteger(input.waitMs) || input.waitMs < 0 || input.waitMs > 60000 ||
    !object(input.value) || !json(input.value) || Buffer.byteLength(JSON.stringify(input)) > 8192) {
    throw new Error('Invalid spike invocation');
  }
  return input as unknown as Request;
}

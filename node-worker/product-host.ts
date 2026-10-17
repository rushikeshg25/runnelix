import { readFileSync } from 'node:fs';
import { Ajv } from 'ajv';
import { modules } from './nodes/index.js';

export interface Envelope {
  contractVersion: 1; workspaceId: string; runId: string; nodeId: string;
  invocationId: string; nodeType: string; nodeVersion: number;
  implementationDigest: string; inputRef: string; configRef: string; logicalActionKey: string;
}
interface Descriptor { type: string; version: number; kind: string; implementationDigest: string; configSchema: object; inputSchema: object; outputSchema: object }
const contractRoot = new URL('../../contracts/product/', import.meta.url);
const ajv = new Ajv({ strict: false });
const validateEnvelope = ajv.compile(JSON.parse(readFileSync(new URL('activity.schema.json', contractRoot), 'utf8')));
const catalog: Descriptor[] = JSON.parse(readFileSync(new URL('catalog.json', contractRoot), 'utf8'));

export class NodeFailure extends Error {
  constructor(public readonly code: 'InvalidInput' | 'VersionMismatch' | 'PayloadUnavailable' | 'InvalidOutput', message: string) { super(message); }
}
export function parseEnvelope(value: unknown): Envelope {
  if (!bounded(value) || !validateEnvelope(value)) throw new NodeFailure('InvalidInput', 'Invalid activity envelope');
  return value as Envelope;
}
// Bounds apply after resolution too. Payload references never authorize access alone.
function bounded(value: unknown, depth = 0): boolean {
  if (depth > 32 || value === undefined) return false;
  if (typeof value === 'number' && (!Number.isFinite(value) || Math.abs(value) > Number.MAX_SAFE_INTEGER)) return false;
  if (value && typeof value === 'object' && !Object.values(value).every(v => bounded(v, depth + 1))) return false;
  try { return Buffer.byteLength(JSON.stringify(value)) <= 8192; } catch { return false; }
}
export interface HostContext {
  signal: AbortSignal;
  // The resolver MUST enforce both workspace and run scope, independently of the reference.
  resolve: (workspaceId: string, runId: string, ref: string) => Promise<unknown>;
}
export async function executeNode(raw: unknown, context: HostContext): Promise<unknown> {
  const task = parseEnvelope(raw);
  const descriptor = catalog.find(n => n.type === task.nodeType && n.version === task.nodeVersion && n.kind === 'action');
  const execute = modules.get(`${task.nodeType}@${task.nodeVersion}`);
  if (!descriptor || !execute || descriptor.implementationDigest !== task.implementationDigest) throw new NodeFailure('VersionMismatch', 'Pinned node implementation unavailable');
  context.signal.throwIfAborted();
  let input: unknown, config: unknown;
  try {
    input = await context.resolve(task.workspaceId, task.runId, task.inputRef);
    config = await context.resolve(task.workspaceId, task.runId, task.configRef);
  } catch { context.signal.throwIfAborted(); throw new NodeFailure('PayloadUnavailable', 'Scoped payload unavailable'); }
  if (!bounded(input) || !bounded(config) || !ajv.validate(descriptor.inputSchema, input) || !ajv.validate(descriptor.configSchema, config)) throw new NodeFailure('InvalidInput', 'Invalid node input or configuration');
  context.signal.throwIfAborted();
  const output = await execute(input, config, context.signal);
  if (!bounded(output) || !ajv.validate(descriptor.outputSchema, output)) throw new NodeFailure('InvalidOutput', 'Invalid node output');
  return output;
}

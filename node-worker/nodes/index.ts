import { postMessage } from './slack.js';
import { createIssue } from './github.js';
// Register trusted internal actions here. The generic host stays unchanged.
import { execute as echo } from './mock-echo.js';
import { get, post } from './http.js';
export interface ApprovedRequest { url: string; method: 'GET' | 'POST'; body?: unknown }
export interface Capabilities { request: (request: ApprovedRequest) => Promise<unknown> }
export type Executor = (input: unknown, config: unknown, signal: AbortSignal, capabilities?: Capabilities) => Promise<unknown>;
export const modules = new Map<string, Executor>([['slack.postMessage@1', postMessage], ['github.createIssue@1', createIssue], ['mock.echo@1', echo], ['http.get@1', get], ['http.post@1', post]]);

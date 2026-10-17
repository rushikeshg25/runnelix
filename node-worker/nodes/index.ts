// Register trusted internal actions here. The generic host stays unchanged.
import { execute as echo } from './mock-echo.js';
export type Executor = (input: unknown, config: unknown, signal: AbortSignal) => Promise<unknown>;
export const modules = new Map<string, Executor>([['mock.echo@1', echo]]);

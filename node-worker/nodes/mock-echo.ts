// Pure fixture node. Does not call a provider or mutate external state.
export async function execute(input: unknown, _config: unknown, signal: AbortSignal): Promise<unknown> {
  signal.throwIfAborted();
  return structuredClone(input);
}

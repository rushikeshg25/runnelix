// Safe, stable error codes. Never include raw provider errors or secrets.
export class NodeFailure extends Error {
  constructor(public readonly code: 'InvalidInput' | 'VersionMismatch' | 'PayloadUnavailable' | 'InvalidOutput' | 'ProviderRejected', message: string) { super(message); }
}

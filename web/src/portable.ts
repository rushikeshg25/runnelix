const maxBytes = 256 * 1024;

export function portableExportText(bundle: unknown): string {
  return JSON.stringify(bundle) + "\n";
}

export function portableImportBody(
  source: string,
  bindings: Record<string, string>,
): string {
  // Check that source is one JSON value, but preserve its original keys for
  // the server's strict decoder instead of silently normalizing duplicates.
  JSON.parse(source);
  const body = `{"bundle":${source},"bindings":${JSON.stringify(bindings)}}`;
  if (new TextEncoder().encode(body).length > maxBytes)
    throw new Error("Workflow import request exceeds 256 KiB.");
  return body;
}

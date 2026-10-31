import { useState } from "react";
import {
  request,
  requestJSON,
  root,
  type Credential,
  type Graph,
  type Workflow,
} from "./api";
import { portableExportText, portableImportBody } from "./portable";
interface Bundle {
  formatVersion: 1;
  name: string;
  graph: Graph;
  credentialSlots: { slot: string; kind: string; destination: string }[];
}
export function PortableWorkflows({
  workflow,
  disabled,
  imported,
}: {
  workflow: Workflow | null;
  disabled: boolean;
  imported: () => Promise<void>;
}) {
  const [source, setSource] = useState("");
  const [bundle, setBundle] = useState<Bundle | null>(null);
  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [credentialCursor, setCredentialCursor] = useState<string | null>(null);
  const [bindings, setBindings] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  async function perform(action: () => Promise<void>) {
    setBusy(true);
    setError("");
    setMessage("");
    try {
      await action();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Request failed");
    } finally {
      setBusy(false);
    }
  }
  function edit(value: string) {
    setSource(value);
    setBundle(null);
    setBindings({});
  }
  async function review() {
    const parsed: Bundle = JSON.parse(source);
    if (
      !parsed ||
      parsed.formatVersion !== 1 ||
      typeof parsed.name !== "string" ||
      !parsed.graph ||
      !Array.isArray(parsed.credentialSlots) ||
      parsed.credentialSlots.length > 100 ||
      parsed.credentialSlots.some(
        (s) =>
          !s ||
          typeof s.slot !== "string" ||
          typeof s.kind !== "string" ||
          typeof s.destination !== "string",
      )
    )
      throw new Error("Choose a version-1 workflow export.");
    const page = await request<Credential[]>(`${root}/credentials`);
    setCredentials(page);
    setCredentialCursor(page.length === 100 ? page.at(-1)!.id : null);
    setBundle(parsed);
    setBindings({});
  }
  async function moreCredentials() {
    if (!credentialCursor) return;
    const page = await request<Credential[]>(
      `${root}/credentials?after=${encodeURIComponent(credentialCursor)}`,
    );
    setCredentials((previous) => [...previous, ...page]);
    setCredentialCursor(page.length === 100 ? page.at(-1)!.id : null);
  }
  async function exportDraft() {
    if (!workflow) return;
    const data = await request<Bundle>(
      `${root}/workflows/${workflow.id}/export`,
    );
    const url = URL.createObjectURL(
      new Blob([portableExportText(data)], {
        type: "application/json",
      }),
    );
    const link = document.createElement("a");
    link.href = url;
    link.download = "workflow.json";
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
    setMessage(
      "Saved draft exported. Review literal configuration before sharing.",
    );
  }
  return (
    <details className="panel portable">
      <summary>Import / export</summary>
      <p className="muted">
        Imports create a new draft. Review before publishing.
      </p>
      {disabled && (
        <p>Save or discard pending edits before importing or exporting.</p>
      )}
      <button
        disabled={disabled || busy || !workflow}
        onClick={() => void perform(exportDraft)}
      >
        Export saved draft
      </button>
      <p className="muted">
        Credential values are excluded. Literal configuration stays in the file.
      </p>
      <label>
        Workflow export file
        <input
          type="file"
          accept=".json,application/json"
          disabled={disabled || busy}
          onChange={(e) => {
            const file = e.target.files?.[0];
            if (!file) return;
            void perform(async () => {
              if (file.size > 256 * 1024)
                throw new Error("Workflow file exceeds 256 KiB.");
              edit(await file.text());
            });
          }}
        />
      </label>
      <label>
        Portable workflow JSON
        <textarea
          rows={5}
          value={source}
          disabled={disabled || busy}
          onChange={(e) => edit(e.target.value)}
        />
      </label>
      <button
        disabled={disabled || busy || !source}
        onClick={() => void perform(review)}
      >
        Review import
      </button>
      {bundle && (
        <div>
          <h4>Import “{bundle.name}”</h4>
          {bundle.credentialSlots.map((slot, index) => (
            <label key={index}>
              {slot.slot} · {slot.destination}
              <select
                value={bindings[slot.slot] ?? ""}
                disabled={disabled || busy}
                onChange={(e) =>
                  setBindings({ ...bindings, [slot.slot]: e.target.value })
                }
              >
                <option value="">Select a local credential</option>
                {credentials
                  .filter(
                    (c) =>
                      !c.revoked &&
                      c.kind === slot.kind &&
                      c.destination === slot.destination,
                  )
                  .map((c) => (
                    <option key={c.id} value={c.id}>
                      {c.name}
                    </option>
                  ))}
              </select>
            </label>
          ))}
          {credentialCursor && bundle.credentialSlots.length > 0 && (
            <button
              disabled={disabled || busy}
              onClick={() => void perform(moreCredentials)}
            >
              Load more credentials
            </button>
          )}
          <button
            disabled={
              disabled ||
              busy ||
              bundle.credentialSlots.some((s) => !bindings[s.slot])
            }
            onClick={() =>
              void perform(async () => {
                await requestJSON<Workflow>(
                  `${root}/workflows/import`,
                  "POST",
                  portableImportBody(source, bindings),
                );
                edit("");
                await imported();
                setMessage(
                  "Workflow imported. Open it from the sidebar to review.",
                );
              })
            }
          >
            Import as new draft
          </button>
        </div>
      )}
      {error && (
        <p role="alert" className="notice error">
          {error}
        </p>
      )}
      {message && (
        <p role="status" className="notice success">
          {message}
        </p>
      )}
    </details>
  );
}

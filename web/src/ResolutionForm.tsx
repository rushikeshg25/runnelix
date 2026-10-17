import { useState } from "react";
import { request, root } from "./api";
export function ResolutionForm({
  runId,
  invocationId,
}: {
  runId: string;
  invocationId: string;
}) {
  const [note, setNote] = useState("");
  const [output, setOutput] = useState("{}");
  const [action, setAction] = useState("confirm");
  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState("");
  return (
    <form
      className="panel"
      onSubmit={(e) => {
        e.preventDefault();
        setBusy(true);
        setError("");
        void (async () => {
          try {
            await request(
              `${root}/runs/${runId}/invocations/${invocationId}/resolve`,
              "POST",
              {
                action,
                note,
                output: action === "confirm" ? JSON.parse(output) : null,
              },
            );
            setSaved(true);
          } catch (error) {
            setError(
              error instanceof Error
                ? error.message
                : "Decision could not be saved.",
            );
          } finally {
            setBusy(false);
          }
        })();
      }}
    >
      <h4>Resolve uncertain action</h4>
      <code>{invocationId}</code>
      <p>
        Check the provider first. This decision is permanent and never sends the
        action again.
      </p>
      {saved ? (
        <p role="status">Decision saved. Waiting for runtime reconciliation.</p>
      ) : (
        <>
          <label>
            Decision
            <select value={action} onChange={(e) => setAction(e.target.value)}>
              <option value="confirm">Confirm verified result</option>
              <option value="fail">End run as failed</option>
            </select>
          </label>
          <label>
            Verification note
            <textarea
              required
              maxLength={500}
              value={note}
              onChange={(e) => setNote(e.target.value)}
            />
          </label>
          {action === "confirm" && (
            <label>
              Verified output (JSON)
              <textarea
                required
                rows={4}
                value={output}
                onChange={(e) => setOutput(e.target.value)}
              />
            </label>
          )}
          <button disabled={busy || !note.trim()}>
            Save permanent decision
          </button>
        </>
      )}
      {error && (
        <p role="alert" className="notice error">
          {error}
        </p>
      )}
    </form>
  );
}

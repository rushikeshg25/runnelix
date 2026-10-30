import { ResolutionForm } from "./ResolutionForm";
import { useEffect, useRef, useState } from "react";
import {
  request,
  APIError,
  sessionExpiredEvent,
  root,
  type Attempt,
  type Run,
  type RunEvent,
  type Version,
  type Workflow,
} from "./api";
export function RunInspector({
  workflow,
  dirty,
}: {
  workflow: Workflow;
  dirty: boolean;
}) {
  const [runs, setRuns] = useState<Run[]>([]);
  const [runId, setRunId] = useState("");
  const [run, setRun] = useState<Run | null>(null);
  const [version, setVersion] = useState<Version | null>(null);
  const [events, setEvents] = useState<RunEvent[]>([]);
  const [attempts, setAttempts] = useState<Attempt[]>([]);
  const [input, setInput] = useState('{"message":"Hello, Runnelix"}');
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [payload, setPayload] = useState<unknown>();
  const pending = useRef<{ key: string; input: string } | null>(null);
  const path = `${root}/workflows/${workflow.id}`;
  const refresh = async () => setRuns(await request<Run[]>(`${path}/runs`));
  useEffect(() => {
    void refresh().catch(() => setError("Run list could not be loaded."));
  }, [path]);
  useEffect(() => {
    if (!runId) return;
    let active = true;
    let inFlight = false;
    let denied = false;
    setEvents([]);
    setVersion(null);
    setPayload(undefined);
    async function inspect() {
      if (inFlight || denied) return;
      inFlight = true;
      try {
        const r = await request<Run>(`${root}/runs/${runId}`);
        const a = await request<Attempt[]>(`${root}/runs/${runId}/attempts`);
        if (active) {
          setRun(r);
          setRuns((previous) =>
            previous.map((value) => (value.runId === r.runId ? r : value)),
          );
          setAttempts(a);
        }
      } catch (error) {
        if (!active) return;
        if (error instanceof APIError && error.status === 410) {
          denied = true;
          stream.close();
          setRunId("");
          setRun(null);
          setVersion(null);
          setEvents([]);
          setAttempts([]);
          setPayload(undefined);
          setRuns((previous) =>
            previous.filter((value) => value.runId !== runId),
          );
          setError("Run history expired under the retention policy.");
          return;
        }
        if (
          error instanceof APIError &&
          (error.status === 401 || error.status === 403)
        ) {
          denied = true;
          stream.close();
          if (active)
            setError(
              "Run inspection requires a valid session and data access.",
            );
        } else if (active) setError("Run inspection disconnected. Retrying…");
      } finally {
        inFlight = false;
      }
    }
    void inspect();
    const stream = new EventSource(`${root}/runs/${runId}/events`);
    stream.onmessage = (event) => {
      const value = JSON.parse(event.data) as RunEvent;
      if (active)
        setEvents((previous) =>
          previous.some((e) => e.sequence === value.sequence)
            ? previous
            : [...previous, value].sort((a, b) => a.sequence - b.sequence),
        );
      void inspect();
    };
    const expire = () => {
      denied = true;
      stream.close();
    };
    window.addEventListener(sessionExpiredEvent, expire);
    const timer = setInterval(() => void inspect(), 2000);
    return () => {
      active = false;
      stream.close();
      window.removeEventListener(sessionExpiredEvent, expire);
      clearInterval(timer);
    };
  }, [runId]);
  useEffect(() => {
    if (!run || run.runId !== runId) return;
    let active = true;
    void request<Version>(`${path}/versions/${run.workflowVersionId}`)
      .then((v) => {
        if (active) setVersion(v);
      })
      .catch(() => {
        if (active) setError("Pinned version could not be loaded.");
      });
    return () => {
      active = false;
    };
  }, [run?.workflowVersionId, runId, path]);
  async function start() {
    setBusy(true);
    setError("");
    try {
      const value = JSON.parse(input);
      if (!pending.current || pending.current.input !== input)
        pending.current = { key: `ui_${crypto.randomUUID()}`, input };
      const result = await request<Run>(
        `${path}/execute`,
        "POST",
        { input: value },
        { "Idempotency-Key": pending.current.key },
      );
      pending.current = null;
      setRun(result);
      setRunId(result.runId);
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Execution request failed");
    } finally {
      setBusy(false);
    }
  }
  const latestInvocations = new Map<string, RunEvent>();
  for (const event of events)
    if (event.invocationId) latestInvocations.set(event.invocationId, event);
  const uncertain = [...latestInvocations.values()].filter(
    (e) => e.nodeState === "uncertain",
  );
  return (
    <section className="panel inspector">
      <div className="panel-title">
        <h3>Run & inspect</h3>
        <span className="muted">Runs use a published version.</span>
      </div>
      <div className="run-start">
        <label>
          Trigger input (JSON)
          <textarea
            rows={3}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            spellCheck={false}
          />
        </label>
        <div>
          <button
            className="primary"
            disabled={busy || dirty || !workflow.activeVersionId}
            onClick={() => void start()}
          >
            Run published version
          </button>
          <small>
            {dirty
              ? "Save and publish your edits first."
              : !workflow.activeVersionId
                ? "Publish this workflow first."
                : "This starts the published workflow’s actions."}
          </small>
        </div>
      </div>
      {error && (
        <p className="notice error" role="alert">
          {error}
        </p>
      )}
      <label>
        Inspect a run
        <select
          value={runId}
          onChange={(e) => {
            setRunId(e.target.value);
            setError("");
          }}
        >
          <option value="">Choose a run…</option>
          {runs.map((r) => (
            <option key={r.runId} value={r.runId}>
              {r.runId.slice(-12)} · {r.state} · {r.executionMode}
            </option>
          ))}
        </select>
      </label>
      {run && run.runId === runId && (
        <>
          <div className="run-summary">
            <span className={`state state-${run.state}`}>
              {run.state.replaceAll("_", " ")}
            </span>
            <code>{run.runId}</code>
            {!["succeeded", "failed", "cancelled"].includes(run.state) &&
              run.executionMode === "execute" && (
                <button
                  disabled={run.cancelRequested}
                  onClick={() => {
                    void request<Run>(`${root}/runs/${runId}/cancel`, "POST")
                      .then(setRun)
                      .catch(() =>
                        setError(
                          "Cancellation request failed. Retry to persist the request.",
                        ),
                      );
                  }}
                >
                  {run.cancelRequested
                    ? "Cancellation requested"
                    : "Cancel run"}
                </button>
              )}
          </div>
          <p className="muted">
            Pinned version {version?.sequence ?? "…"} · draft revision{" "}
            {version?.draftRevision ?? "…"} · checkpoint {run.checkpoint}
          </p>
          {run.state === "needs_attention" && (
            <p className="notice error">
              The provider outcome is uncertain. Do not repeat this action until
              you check the provider record.
            </p>
          )}
          {run.state === "needs_attention" &&
            uncertain.map((e) => (
              <ResolutionForm
                key={`${runId}/${e.invocationId}`}
                runId={runId}
                invocationId={e.invocationId!}
              />
            ))}
          <div className="inspection-columns">
            <div>
              <h4>Execution timeline</h4>
              <ol className="timeline">
                {events.map((e) => (
                  <li key={e.sequence}>
                    <span className="event-number">{e.sequence}</span>
                    <div>
                      <strong>
                        {e.nodeId ?? "Run"} · {e.nodeState ?? e.state}
                      </strong>
                      {e.invocationId && (
                        <small>
                          <code>{e.invocationId}</code>
                        </small>
                      )}
                      {e.errorCode && <small>{e.errorCode}</small>}
                      {e.outputRef && (
                        <button
                          onClick={() => {
                            void request(
                              `${root}/runs/${runId}/payloads/${e.outputRef}`,
                            )
                              .then(setPayload)
                              .catch((error: unknown) =>
                                setError(
                                  error instanceof APIError &&
                                    error.status === 410
                                    ? "Output expired under the retention policy."
                                    : "Output could not be loaded.",
                                ),
                              );
                          }}
                        >
                          View output
                        </button>
                      )}
                    </div>
                  </li>
                ))}
              </ol>
            </div>
            <div>
              <h4>Pinned graph</h4>
              <ul>
                {version?.graph.nodes.map((n) => (
                  <li key={n.id}>
                    <code>{n.id}</code> · {n.type}@{n.version}
                  </li>
                ))}
              </ul>
              <h4>Recorded attempts</h4>
              {attempts.length ? (
                <ul>
                  {attempts.map((a) => (
                    <li key={`${a.invocationId}/${a.attempt}`}>
                      <code>{a.invocationId}</code> · attempt {a.attempt}:{" "}
                      {a.status}
                      {a.errorCode ? ` (${a.errorCode})` : ""}
                    </li>
                  ))}
                </ul>
              ) : (
                <p className="muted">No action attempt recorded yet.</p>
              )}
              {payload !== undefined && (
                <>
                  <h4>Selected output</h4>
                  <pre>{JSON.stringify(payload, null, 2)}</pre>
                </>
              )}
            </div>
          </div>
        </>
      )}
    </section>
  );
}

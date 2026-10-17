import { useEffect, useState } from "react";
import { request, root, type Workflow } from "./api";
interface Spec {
  kind: string;
  intervalSeconds: number;
  hour: number;
  minute: number;
  timezone: string;
  enabled: boolean;
  input: Record<string, unknown>;
}
interface Schedule {
  id: string;
  workflowId: string;
  revision: number;
  observedRevision: number;
  spec: Spec;
}
export function Schedules({ workflow }: { workflow: Workflow }) {
  const [schedules, setSchedules] = useState<Schedule[]>([]);
  const [kind, setKind] = useState("interval");
  const [seconds, setSeconds] = useState(3600);
  const [time, setTime] = useState("09:00");
  const [zone, setZone] = useState("UTC");
  const [input, setInput] = useState("{}");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  async function refresh() {
    setSchedules(
      (await request<Schedule[]>(`${root}/schedules`)).filter(
        (s) => s.workflowId === workflow.id,
      ),
    );
  }
  useEffect(() => {
    let active = true;
    const poll = async () => {
      try {
        const values = await request<Schedule[]>(`${root}/schedules`);
        if (active)
          setSchedules(values.filter((s) => s.workflowId === workflow.id));
      } catch {
        if (active) setError("Schedule status could not be loaded.");
      }
    };
    void poll();
    const timer = setInterval(() => void poll(), 3000);
    return () => {
      active = false;
      clearInterval(timer);
    };
  }, [workflow.id]);
  async function save(schedule?: Schedule) {
    setBusy(true);
    setError("");
    try {
      const [hour, minute] = time.split(":").map(Number);
      const spec: Spec = schedule
        ? { ...schedule.spec, enabled: !schedule.spec.enabled }
        : {
            kind,
            intervalSeconds: kind === "interval" ? seconds : 0,
            hour,
            minute,
            timezone: kind === "interval" ? "UTC" : zone,
            enabled: false,
            input: JSON.parse(input),
          };
      await request(
        `${root}/schedules${schedule ? `/${schedule.id}` : ""}`,
        schedule ? "PUT" : "POST",
        { workflowId: workflow.id, spec },
        schedule ? { "If-Match": `"${schedule.revision}"` } : {},
      );
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Schedule could not be saved.");
    } finally {
      setBusy(false);
    }
  }
  return (
    <details className="panel">
      <summary>Schedules · {schedules.length}</summary>
      <p>
        Schedules run the active published version. Overlapping runs are
        skipped. Missed occurrences have a one-minute catch-up window.
      </p>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void save();
        }}
      >
        <label>
          Schedule type
          <select value={kind} onChange={(e) => setKind(e.target.value)}>
            <option value="interval">Interval</option>
            <option value="daily">Daily local time</option>
          </select>
        </label>
        {kind === "interval" ? (
          <label>
            Interval seconds
            <input
              type="number"
              min={60}
              max={86400}
              required
              value={seconds}
              onChange={(e) => setSeconds(Number(e.target.value))}
            />
          </label>
        ) : (
          <>
            <label>
              Daily time
              <input
                type="time"
                required
                value={time}
                onChange={(e) => setTime(e.target.value)}
              />
            </label>
            <label>
              IANA timezone
              <input
                required
                value={zone}
                onChange={(e) => setZone(e.target.value)}
              />
            </label>
            <small>
              Missing daylight-saving times are skipped. A repeated local time
              runs at most once.
            </small>
          </>
        )}
        <label>
          Scheduled input (JSON)
          <textarea
            value={input}
            onChange={(e) => setInput(e.target.value)}
            required
          />
        </label>
        <button disabled={busy || !workflow.activeVersionId}>
          Create paused schedule
        </button>
        {!workflow.activeVersionId && <small>Publish a version first.</small>}
      </form>
      {error && (
        <p role="alert" className="notice error">
          {error}
        </p>
      )}
      <ul>
        {schedules.map((s) => (
          <li key={s.id}>
            <code>{s.id}</code>
            <p>
              {s.spec.kind === "interval"
                ? `Every ${s.spec.intervalSeconds} seconds`
                : `${String(s.spec.hour).padStart(2, "0")}:${String(s.spec.minute).padStart(2, "0")} ${s.spec.timezone}`}{" "}
              · {s.spec.enabled ? "Enabled" : "Paused"} ·{" "}
              {s.observedRevision === s.revision
                ? "Synchronized"
                : "Synchronization pending"}
            </p>
            <button disabled={busy} onClick={() => void save(s)}>
              {s.spec.enabled ? "Pause schedule" : "Enable schedule"}
            </button>
          </li>
        ))}
      </ul>
    </details>
  );
}

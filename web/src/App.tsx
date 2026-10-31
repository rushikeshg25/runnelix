import { useEffect, useState, type ReactNode } from "react";
import {
  APIError,
  emptyGraph,
  request,
  root,
  type Descriptor,
  type Graph,
  type Workflow,
} from "./api";
import { SchemaForm } from "./SchemaForm";
import { Canvas } from "./Canvas";
import { RunInspector } from "./RunInspector";
import { PortableWorkflows } from "./PortableWorkflows";
import { Schedules } from "./Schedules";
import { ThemePicker } from "./Theme";
import { MappingPreview } from "./MappingPreview";
export default function App({
  sessionActions,
}: {
  sessionActions?: ReactNode;
}) {
  const [catalog, setCatalog] = useState<Descriptor[]>([]);
  const [workflows, setWorkflows] = useState<Workflow[]>([]);
  const [current, setCurrent] = useState<Workflow | null>(null);
  const [graph, setGraph] = useState<Graph>(emptyGraph);
  const [selected, setSelected] = useState("echo");
  const [addType, setAddType] = useState("mock.echo");
  const [name, setName] = useState("My first workflow");
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const dirty =
    current !== null && JSON.stringify(current.graph) !== JSON.stringify(graph);
  const path = current ? `${root}/workflows/${current.id}` : "";
  async function perform(action: () => Promise<void>) {
    setBusy(true);
    setError("");
    try {
      await action();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Request failed");
    } finally {
      setBusy(false);
    }
  }
  async function refresh() {
    setWorkflows(await request<Workflow[]>(`${root}/workflows`));
  }
  useEffect(() => {
    void perform(async () => {
      setCatalog(await request<Descriptor[]>("/v1/node-types"));
      await refresh();
    });
  }, []);
  useEffect(() => {
    const warn = (event: BeforeUnloadEvent) => {
      if (dirty) event.preventDefault();
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty]);
  function open(w: Workflow) {
    if (dirty && !window.confirm("Discard unsaved changes?")) return;
    setCurrent(w);
    setGraph(w.graph);
    setSelected(w.graph.nodes[0]?.id ?? "");
    setMessage("");
    setError("");
  }
  async function create() {
    const w = await request<Workflow>(`${root}/workflows`, "POST", {
      name,
      graph: emptyGraph(),
    });
    open(w);
    await refresh();
    setMessage("");
  }
  async function save() {
    if (!current) return;
    try {
      const w = await request<Workflow>(
        `${path}/draft`,
        "PUT",
        { graph },
        { "If-Match": `"${current.revision}"` },
      );
      setCurrent(w);
      setGraph(w.graph);
      await refresh();
      setMessage("");
    } catch (e) {
      if (e instanceof APIError && e.status === 409)
        throw new Error(
          "Another edit changed this workflow. Your edits are still here. Copy the graph before reloading the saved draft.",
        );
      throw e;
    }
  }
  function addNode() {
    const d = catalog.find((n) => n.type === addType);
    if (!d) return;
    const id = `node_${crypto.randomUUID().replaceAll("-", "").slice(0, 10)}`;
    const config: Record<string, unknown> = d.configSchema.properties
      ?.expression
      ? { expression: d.type === "control.condition" ? "true" : "input" }
      : {};
    if (d.type === "control.merge") Object.assign(config, { policy: "named" });
    if (d.type === "control.wait") Object.assign(config, { seconds: 60 });
    if (d.type === "control.map")
      Object.assign(config, {
        collectionExpression: "input.items",
        bodyType: "mock.echo",
        bodyVersion: 1,
        bodyConfig: {},
        maxItems: 100,
        concurrency: 4,
      });
    setGraph({
      ...graph,
      nodes: [...graph.nodes, { id, type: d.type, version: d.version, config }],
    });
    setSelected(id);
  }
  const node = graph.nodes.find((n) => n.id === selected);
  const descriptor = catalog.find(
    (d) => d.type === node?.type && d.version === node.version,
  );
  return (
    <div className="app">
      <header className="topbar">
        <a className="brand" href="/" aria-label="Runnelix home">
          <span className="brand-icon">↝</span>runnelix
        </a>
        <div className="session-actions">
          <span className="local-badge">● Local workspace</span>
          <ThemePicker />
          {sessionActions}
        </div>
      </header>
      <div className="layout">
        <aside className="sidebar">
          <h1>Workflows</h1>
          <details className="new-workflow">
            <summary>New workflow</summary>
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void perform(create);
              }}
            >
              <label>
                Workflow name
                <input
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  maxLength={120}
                  required
                />
              </label>
              <button className="primary wide" disabled={busy || dirty}>
                + Create workflow
              </button>
            </form>
          </details>
          <nav aria-label="Workflows">
            {workflows.map((w) => (
              <button
                className={`workflow-item ${current?.id === w.id ? "active" : ""}`}
                key={w.id}
                aria-current={current?.id === w.id ? "page" : undefined}
                disabled={busy}
                onClick={() => open(w)}
              >
                <span>{w.name}</span>
                <small>
                  {w.activeVersionId ? "Published" : "Draft"} · r{w.revision}
                </small>
              </button>
            ))}
          </nav>
          {workflows.length >= 50 && (
            <button
              onClick={() =>
                void perform(async () =>
                  setWorkflows([
                    ...workflows,
                    ...(await request<Workflow[]>(
                      `${root}/workflows?after=${workflows.at(-1)!.id}`,
                    )),
                  ]),
                )
              }
            >
              Load more
            </button>
          )}
        </aside>
        <main>
          <div role="alert" className={error ? "notice error" : "hidden"}>
            {error}
          </div>
          <div role="status" className={message ? "notice success" : "hidden"}>
            {message}
          </div>
          {!current ? (
            <section className="welcome">
              <h2>Choose a workflow</h2>
              <p>
                Open a workflow from the sidebar, or create one to get started.
              </p>
            </section>
          ) : (
            <>
              <section className="workflow-header">
                <div>
                  <h2>{current.name}</h2>
                  <span className="muted">
                    {dirty
                      ? "Unsaved changes"
                      : `Draft ${current.revision} saved`}
                  </span>
                </div>
                <div className="actions">
                  <button
                    disabled={busy || !dirty}
                    onClick={() => void perform(save)}
                  >
                    Save draft
                  </button>
                  <button
                    disabled={busy || dirty}
                    onClick={() =>
                      void perform(async () => {
                        await request(`${path}/validate`, "POST");
                        setMessage("Workflow is valid.");
                      })
                    }
                  >
                    Validate
                  </button>
                  <button
                    className="primary"
                    disabled={busy || dirty}
                    onClick={() =>
                      void perform(async () => {
                        await request(`${path}/publish`, "POST", undefined, {
                          "If-Match": `"${current.revision}"`,
                        });
                        const w = await request<Workflow>(path);
                        setCurrent(w);
                        await refresh();
                        setMessage("Workflow published.");
                      })
                    }
                  >
                    Publish
                  </button>
                </div>
              </section>
              <section className="editor-grid">
                <div className="canvas-column">
                  <Canvas
                    key={`canvas-${current.id}`}
                    workflowId={current.id}
                    graph={graph}
                    catalog={catalog}
                    selected={selected}
                    select={setSelected}
                    change={setGraph}
                  />
                  <div className="panel steps-panel">
                    <div className="panel-title">
                      <h3>Steps</h3>
                    </div>
                    <div className="node-list">
                      {graph.nodes.map((n) => (
                        <button
                          key={n.id}
                          onClick={() => setSelected(n.id)}
                          aria-pressed={selected === n.id}
                          className={selected === n.id ? "selected" : ""}
                        >
                          <span className="node-symbol">
                            {n.type.startsWith("trigger.")
                              ? "↳"
                              : n.type.startsWith("control.")
                                ? "◇"
                                : "▤"}
                          </span>
                          <span>
                            <strong>
                              {catalog.find((d) => d.type === n.type)?.title ??
                                n.type}
                            </strong>
                            <small>{n.id}</small>
                          </span>
                        </button>
                      ))}
                    </div>
                    <div className="add-node">
                      <label>
                        Step type
                        <select
                          value={addType}
                          onChange={(e) => setAddType(e.target.value)}
                        >
                          {catalog
                            .filter((d) => d.kind !== "trigger")
                            .map((d) => (
                              <option key={d.type} value={d.type}>
                                {d.title}
                              </option>
                            ))}
                        </select>
                      </label>
                      <button onClick={addNode}>Add step</button>
                    </div>
                  </div>
                </div>
                <div className="panel configuration">
                  <div className="panel-title">
                    <h3>{descriptor?.title ?? "Step settings"}</h3>
                    <span className="muted">{node?.id}</span>
                  </div>
                  {node && descriptor && (
                    <>
                      <SchemaForm
                        descriptor={descriptor}
                        node={node}
                        change={(config) =>
                          setGraph({
                            ...graph,
                            nodes: graph.nodes.map((n) =>
                              n.id === node.id ? { ...n, config } : n,
                            ),
                          })
                        }
                      />
                      {typeof node.config.expression === "string" && (
                        <MappingPreview expression={node.config.expression} />
                      )}
                      <details className="step-details">
                        <summary>Step details</summary>
                        <p className="muted">
                          Type: {node.type} v{node.version}
                          <br />
                          Retry safety: {descriptor.retrySafety}
                        </p>
                      </details>
                      {descriptor.kind !== "trigger" && (
                        <button
                          className="danger"
                          onClick={() => {
                            setGraph({
                              ...graph,
                              nodes: graph.nodes.filter(
                                (n) => n.id !== node.id,
                              ),
                              edges: graph.edges.filter(
                                (e) =>
                                  e.from.node !== node.id &&
                                  e.to.node !== node.id,
                              ),
                            });
                            setSelected("start");
                          }}
                        >
                          Remove step
                        </button>
                      )}
                    </>
                  )}
                </div>
              </section>
              <Schedules key={`schedules-${current.id}`} workflow={current} />
              <RunInspector
                key={`inspector-${current.id}`}
                workflow={current}
                dirty={dirty}
              />
              <details className="panel raw">
                <summary>Workflow JSON</summary>
                <textarea
                  aria-label="Workflow JSON"
                  readOnly
                  value={JSON.stringify(graph, null, 2)}
                  rows={12}
                />
              </details>
            </>
          )}
          <PortableWorkflows
            workflow={current}
            disabled={busy || dirty}
            imported={refresh}
          />
        </main>
      </div>
    </div>
  );
}

import { useCallback, useMemo, useState } from "react";
import {
  ReactFlow,
  Background,
  Controls,
  Handle,
  Position,
  type NodeProps,
  type Node,
  type Connection,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import type { Descriptor, Graph } from "./api";
type StepNode = Node<
  { title: string; inputs: string[]; outputs: string[]; kind: string },
  "step"
>;
function Step({ data, selected }: NodeProps<StepNode>) {
  return (
    <div className={`flow-step ${selected ? "focused" : ""}`}>
      <div className="flow-kind">{data.kind}</div>
      <strong>{data.title}</strong>
      {data.inputs.map((port, i) => (
        <Handle
          type="target"
          key={port}
          id={port}
          position={Position.Left}
          style={{ top: `${((i + 1) * 100) / (data.inputs.length + 1)}%` }}
          aria-label={`Input ${port}`}
        />
      ))}
      {data.outputs.map((port, i) => (
        <Handle
          type="source"
          key={port}
          id={port}
          position={Position.Right}
          style={{ top: `${((i + 1) * 100) / (data.outputs.length + 1)}%` }}
          aria-label={`Output ${port}`}
        />
      ))}
      <small>{data.outputs.join(" / ")}</small>
    </div>
  );
}
const nodeTypes = { step: Step };
export function Canvas({
  graph,
  catalog,
  selected,
  select,
  change,
  workflowId,
}: {
  graph: Graph;
  catalog: Descriptor[];
  selected: string;
  select: (id: string) => void;
  change: (graph: Graph) => void;
  workflowId: string;
}) {
  const storageKey = `runnelix.positions.${workflowId}`;
  const [positions, setPositions] = useState<
    Record<string, { x: number; y: number }>
  >(() => {
    try {
      return JSON.parse(localStorage.getItem(storageKey) ?? "{}");
    } catch {
      return {};
    }
  });
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  // React Flow selects on drag start. Keep the controlled selection in sync
  // before position updates feed nodes back into the canvas.
  const selectNode = useCallback(
    (_event: unknown, node: StepNode) => select(node.id),
    [select],
  );
  const nodes = useMemo<StepNode[]>(
    () =>
      graph.nodes.map((n, i) => {
        const d = catalog.find((d) => d.type === n.type);
        return {
          id: n.id,
          type: "step",
          selected: n.id === selected,
          position: positions[n.id] ?? {
            x: (i % 3) * 260 + 30,
            y: Math.floor(i / 3) * 130 + 60,
          },
          data: {
            title: d?.title ?? n.type,
            kind: d?.kind ?? "node",
            inputs: d?.inputs ?? [],
            outputs: d?.outputs ?? [],
          },
        };
      }),
    [graph.nodes, catalog, selected, positions],
  );
  function connect(c: Connection) {
    if (!c.sourceHandle || !c.targetHandle || c.source === c.target) return;
    change({
      ...graph,
      edges: [
        ...graph.edges.filter(
          (e) => !(e.to.node === c.target && e.to.port === c.targetHandle),
        ),
        {
          from: { node: c.source, port: c.sourceHandle },
          to: { node: c.target, port: c.targetHandle },
        },
      ],
    });
  }
  return (
    <section className="panel canvas-panel">
      <div className="panel-title">
        <h3>Workflow canvas</h3>
        <span className="muted">
          Drag between ports, or use the connection form below.
        </span>
      </div>
      <div className="canvas" aria-label="Workflow canvas">
        <ReactFlow
          nodes={nodes}
          edges={graph.edges.map((e, i) => ({
            id: String(i),
            source: e.from.node,
            sourceHandle: e.from.port,
            target: e.to.node,
            targetHandle: e.to.port,
            label: e.from.port,
            animated: false,
          }))}
          nodeTypes={nodeTypes}
          onNodeClick={selectNode}
          onNodeDragStart={selectNode}
          onConnect={connect}
          fitView
          minZoom={0.2}
          maxZoom={1.8}
          deleteKeyCode={null}
          onNodesChange={(changes) => {
            const next = { ...positions };
            let changed = false;
            for (const c of changes)
              if (c.type === "position" && c.position) {
                next[c.id] = c.position;
                changed = true;
              }
            if (changed) {
              setPositions(next);
              try {
                localStorage.setItem(storageKey, JSON.stringify(next));
              } catch {
                /* Layout storage is optional; graph edits remain in memory. */
              }
            }
          }}
        >
          <Background gap={20} color="#d7e0e5" />
          <Controls showInteractive={false} />
        </ReactFlow>
      </div>
      <form
        className="connections"
        onSubmit={(e) => {
          e.preventDefault();
          const [source, sourceHandle] = from.split("/");
          const [target, targetHandle] = to.split("/");
          if (source && target && sourceHandle && targetHandle)
            connect({ source, sourceHandle, target, targetHandle });
        }}
      >
        <label>
          From output
          <select
            value={from}
            onChange={(e) => setFrom(e.target.value)}
            required
          >
            <option value="">Choose output…</option>
            {graph.nodes.flatMap((n) =>
              (catalog.find((d) => d.type === n.type)?.outputs ?? []).map(
                (p) => (
                  <option key={`${n.id}/${p}`} value={`${n.id}/${p}`}>
                    {n.id} → {p}
                  </option>
                ),
              ),
            )}
          </select>
        </label>
        <label>
          To input
          <select value={to} onChange={(e) => setTo(e.target.value)} required>
            <option value="">Choose input…</option>
            {graph.nodes.flatMap((n) =>
              (catalog.find((d) => d.type === n.type)?.inputs ?? []).map(
                (p) => (
                  <option key={`${n.id}/${p}`} value={`${n.id}/${p}`}>
                    {n.id} ← {p}
                  </option>
                ),
              ),
            )}
          </select>
        </label>
        <button>Connect</button>
      </form>
      <ul className="edge-list">
        {graph.edges.map((e, i) => (
          <li key={`${e.from.node}/${e.from.port}/${e.to.node}/${e.to.port}`}>
            <span>
              {e.from.node}.{e.from.port} → {e.to.node}.{e.to.port}
            </span>
            <button
              aria-label={`Remove connection ${e.from.node} to ${e.to.node}`}
              onClick={() =>
                change({
                  ...graph,
                  edges: graph.edges.filter((_, j) => i !== j),
                })
              }
            >
              Remove
            </button>
          </li>
        ))}
      </ul>
    </section>
  );
}

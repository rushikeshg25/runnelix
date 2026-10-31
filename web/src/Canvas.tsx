import { memo, useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ReactFlow,
  Background,
  Controls,
  Handle,
  Position,
  type NodeProps,
  type Node,
  type Connection,
  type NodeChange,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import type { Descriptor, Graph } from "./api";
type StepNode = Node<
  { title: string; inputs: string[]; outputs: string[]; kind: string },
  "step"
>;
const Step = memo(function Step({ data, selected }: NodeProps<StepNode>) {
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
          title={`Input: ${port}`}
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
          title={`Output: ${port}`}
        />
      ))}
      {data.outputs.length > 1 && <small>{data.outputs.join(" / ")}</small>}
    </div>
  );
});
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
  const savedPositions = useRef(positions);
  const [dragging, setDragging] = useState(false);
  useEffect(() => {
    if (dragging || positions === savedPositions.current) return;
    try {
      localStorage.setItem(storageKey, JSON.stringify(positions));
      savedPositions.current = positions;
    } catch {
      // Layout storage is optional; graph edits remain in memory.
    }
  }, [dragging, positions, storageKey]);
  const moveNodes = useCallback((changes: NodeChange<StepNode>[]) => {
    const moves = changes.filter((c) => c.type === "position" && c.position);
    if (!moves.length) return;
    setPositions((previous) => {
      const next = { ...previous };
      for (const move of moves)
        if (move.type === "position" && move.position)
          next[move.id] = move.position;
      return next;
    });
  }, []);
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  // React Flow selects on drag start. Keep the controlled selection in sync
  // before position updates feed nodes back into the canvas.
  const selectNode = useCallback(
    (_event: unknown, node: StepNode) => select(node.id),
    [select],
  );
  const definitions = useMemo<StepNode[]>(
    () =>
      graph.nodes.map((n, i) => {
        const d = catalog.find((d) => d.type === n.type);
        return {
          id: n.id,
          type: "step",
          position: {
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
    [graph.nodes, catalog],
  );
  const nodes = useMemo(
    () =>
      definitions.map((node) => ({
        ...node,
        selected: node.id === selected,
        position: positions[node.id] ?? node.position,
      })),
    [definitions, selected, positions],
  );
  const edges = useMemo(
    () =>
      graph.edges.map((edge) => ({
        id: `${edge.from.node}/${edge.from.port}/${edge.to.node}/${edge.to.port}`,
        source: edge.from.node,
        sourceHandle: edge.from.port,
        target: edge.to.node,
        targetHandle: edge.to.port,
        label: edge.from.port,
      })),
    [graph.edges],
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
          edges={edges}
          nodeTypes={nodeTypes}
          onNodeClick={selectNode}
          onNodeDragStart={(event, node) => {
            setDragging(true);
            selectNode(event, node);
          }}
          onNodeDragStop={() => setDragging(false)}
          onConnect={connect}
          fitView
          minZoom={0.2}
          maxZoom={1.8}
          deleteKeyCode={null}
          onNodesChange={moveNodes}
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

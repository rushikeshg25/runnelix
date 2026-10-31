// Serve with: pnpm exec vite . --config web/vite.config.ts --port 5174
// Open /tests/web/canvas.html and run the checks. Uses the real Canvas and
// React Flow event handlers, with fixture data and no backend requests.
import React, { useState } from "react";
import { createRoot } from "react-dom/client";
import { ThemeProvider, ThemePicker } from "../../web/src/Theme";
import { Canvas } from "../../web/src/Canvas";
import { emptyGraph, type Descriptor } from "../../web/src/api";
import catalog from "../../contracts/product/catalog.json";
import "../../web/src/style.css";

const frame = () =>
  new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
async function settled() {
  await frame();
  await frame();
}

function Checks() {
  const [graph, setGraph] = useState(emptyGraph);
  const [selected, setSelected] = useState("start");
  const [result, setResult] = useState("Ready");
  async function checkDrag() {
    setResult("Running");
    await settled();
    const node = document.querySelector<HTMLElement>(
      '.react-flow__node[data-id="echo"]',
    )!;
    const box = node.getBoundingClientRect();
    const x = box.x + box.width / 2;
    const y = box.y + box.height / 2;
    const samples: {
      x: number;
      dragging: boolean;
      selected: boolean;
      width: number;
      height: number;
    }[] = [];
    const mouse = (target: EventTarget, type: string, dx: number) =>
      target.dispatchEvent(
        new MouseEvent(type, {
          bubbles: true,
          cancelable: true,
          view: window,
          clientX: x + dx,
          clientY: y,
          button: 0,
          buttons: type === "mouseup" ? 0 : 1,
        }),
      );
    const originalSetItem = Storage.prototype.setItem;
    let layoutWrites = 0;
    Storage.prototype.setItem = function (key, value) {
      if (key === "runnelix.positions.canvas-regression-fixture")
        layoutWrites++;
      return originalSetItem.call(this, key, value);
    };
    mouse(node, "mousedown", 0);
    for (let i = 1; i <= 16; i++) {
      mouse(window, "mousemove", i * 4);
      await settled();
      samples.push({
        x: node.getBoundingClientRect().x,
        width: node.getBoundingClientRect().width,
        height: node.getBoundingClientRect().height,
        dragging: node.classList.contains("dragging"),
        selected: node.classList.contains("selected"),
      });
    }
    mouse(window, "mouseup", 64);
    await settled();
    Storage.prototype.setItem = originalSetItem;
    const failures: string[] = [];
    if (layoutWrites !== 1)
      failures.push(
        `Expected one layout write after release, received ${layoutWrites}`,
      );
    if (samples.some((s) => Math.abs(s.height - box.height) > 0.5))
      failures.push("Selection changes node height and moves its ports");
    if (samples.some((s) => !s.dragging))
      failures.push("Drag state resets during pointer movement");
    if (samples.some((s) => !s.selected))
      failures.push("Dragged node loses selection");
    if (samples.some((s, i) => i > 0 && s.x < samples[i - 1].x - 0.5))
      failures.push("Node jumps backwards");
    if (samples.at(-1)!.x < box.x + 50)
      failures.push("Node does not follow the pointer");
    if (node.classList.contains("dragging"))
      failures.push("Drag state remains after release");
    setResult(
      JSON.stringify(
        {
          status: failures.length ? "FAIL" : "PASS",
          failures,
          layoutWrites,
          samples,
        },
        null,
        2,
      ),
    );
  }
  return (
    <main style={{ maxWidth: 1000, margin: "auto" }}>
      <h1>Canvas regression checks</h1>
      <ThemePicker />
      <button onClick={() => void checkDrag()}>Check drag stability</button>
      <pre aria-label="Check result">{result}</pre>
      <Canvas
        workflowId="canvas-regression-fixture"
        graph={graph}
        catalog={catalog as Descriptor[]}
        selected={selected}
        select={setSelected}
        change={setGraph}
      />
    </main>
  );
}
createRoot(document.getElementById("root")!).render(
  <ThemeProvider>
    <Checks />
  </ThemeProvider>,
);

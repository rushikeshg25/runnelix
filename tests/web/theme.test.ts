import assert from "node:assert/strict";
import { test } from "node:test";
import { parseTheme, readTheme, resolveTheme } from "../../web/src/theme.ts";

test("theme preferences default to system and preserve explicit choices", () => {
  for (const value of [null, "", "system", "invalid"])
    assert.equal(parseTheme(value), "system");
  for (const value of ["light", "dark"] as const)
    assert.equal(parseTheme(value), value);
  assert.equal(resolveTheme("system", true), "dark");
  assert.equal(resolveTheme("system", false), "light");
  assert.equal(resolveTheme("light", true), "light");
  assert.equal(resolveTheme("dark", false), "dark");
});

test("blocked browser storage falls back to the system theme", (t) => {
  const previous = Object.getOwnPropertyDescriptor(globalThis, "localStorage");
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    get() {
      throw new Error("Storage denied");
    },
  });
  t.after(() => {
    if (previous) Object.defineProperty(globalThis, "localStorage", previous);
    else delete globalThis.localStorage;
  });
  assert.equal(readTheme(), "system");
});

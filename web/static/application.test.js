import { describe, it } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";

describe("application.js", () => {
  it("contains required HTMX handlers and safe transport rendering", () => {
    const script = fs.readFileSync(new URL("./application.js", import.meta.url), "utf8");
    for (const expected of [
      "htmx:beforeSwap",
      "htmx:sendError",
      "htmx:timeout",
      "event.detail.shouldSwap = true",
      'startsWith("text/html")',
      "GoPanel could not be reached.",
      "GoPanel did not respond in time.",
      "replaceChildren",
    ]) {
      assert.ok(script.includes(expected), `expected ${expected}`);
    }
    for (const forbidden of ["innerHTML", "insertAdjacentHTML", "Error reference"]) {
      assert.ok(!script.includes(forbidden), `forbidden ${forbidden}`);
    }
  });
});

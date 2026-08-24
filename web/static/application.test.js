import { describe, it } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";

describe("application.js", () => {
  it("swaps only HTML error responses", () => {
    const runtime = loadApplication();
    const htmlError = responseEvent(500, "text/html; charset=utf-8");
    const jsonError = responseEvent(500, "application/json");
    const htmlSuccess = responseEvent(200, "text/html");

    runtime.listeners.get("htmx:beforeSwap")(htmlError);
    runtime.listeners.get("htmx:beforeSwap")(jsonError);
    runtime.listeners.get("htmx:beforeSwap")(htmlSuccess);

    assert.equal(htmlError.detail.shouldSwap, true);
    assert.equal(jsonError.detail.shouldSwap, false);
    assert.equal(htmlSuccess.detail.shouldSwap, false);
  });

  it("renders fixed safe connection and timeout failures", () => {
    const runtime = loadApplication();
    const connectionTarget = new runtime.Element();
    const timeoutTarget = new runtime.Element();

    runtime.listeners.get("htmx:sendError")({ detail: { target: connectionTarget } });
    runtime.listeners.get("htmx:timeout")({ detail: { target: timeoutTarget } });

    assert.equal(connectionTarget.children.length, 1);
    assert.equal(timeoutTarget.children.length, 1);
    assert.match(flattenText(connectionTarget), /GoPanel could not be reached/);
    assert.match(flattenText(timeoutTarget), /GoPanel did not respond in time/);
    assert.doesNotMatch(flattenText(connectionTarget), /Error reference/);
    assert.equal(connectionTarget.children[0].attributes.get("role"), "alert");
  });
});

function loadApplication() {
  const listeners = new Map();
  class Element {
    constructor() {
      this.attributes = new Map();
      this.children = [];
      this.textContent = "";
      this.className = "";
      this.href = "";
    }

    append(...children) {
      this.children.push(...children);
    }

    closest() {
      return this;
    }

    replaceChildren(...children) {
      this.children = children;
    }

    setAttribute(name, value) {
      this.attributes.set(name, value);
    }
  }
  const document = {
    addEventListener(name, handler) {
      listeners.set(name, handler);
    },
    createElement() {
      return new Element();
    },
  };
  const source = fs.readFileSync(new URL("./application.js", import.meta.url), "utf8");
  vm.runInNewContext(source, { document, Element });
  return { Element, listeners };
}

function responseEvent(status, contentType) {
  return {
    detail: {
      shouldSwap: false,
      xhr: {
        status,
        getResponseHeader() {
          return contentType;
        },
      },
    },
  };
}

function flattenText(element) {
  return [element.textContent, ...element.children.map(flattenText)].join(" ");
}

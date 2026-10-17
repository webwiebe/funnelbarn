// The ratchet's own test. A gate that has quietly stopped failing looks exactly
// like a clean repo, which is why it is tested before it is trusted. `compare`
// and `flatten` are the whole decision, so they are the whole test surface:
// running knip itself is not what can silently break.

import test from "node:test";
import assert from "node:assert/strict";
import { analysedFiles, checkFloor, compare, flatten, refreshCommand, ITEM_TYPES, MIN_FILES } from "./knip-ratchet.mjs";

test("a clean tree against an empty baseline passes", () => {
  const res = compare([], []);
  assert.equal(res.ok, true);
  assert.equal(res.total, 0);
});

test("a NEW dead item fails and is named", () => {
  const res = compare([], ["exports:src/a.ts:foo"]);
  assert.equal(res.ok, false, "new dead code must fail the ratchet");
  assert.deepEqual(res.added, ["exports:src/a.ts:foo"]);
  assert.deepEqual(res.removed, []);
});

test("an item on the baseline passes", () => {
  const items = ["exports:src/a.ts:foo", "files:src/b.ts:"];
  assert.equal(compare(items, items).ok, true);
});

test("an item that is gone fails, so the baseline has to follow the work down", () => {
  const res = compare(["exports:src/a.ts:foo", "exports:src/a.ts:bar"], ["exports:src/a.ts:bar"]);
  assert.equal(res.ok, false, "a stale allowance must fail, not pass quietly");
  assert.deepEqual(res.removed, ["exports:src/a.ts:foo"]);
  assert.deepEqual(res.added, []);
});

test("swapping one item for another reports both sides", () => {
  const res = compare(["exports:src/a.ts:foo"], ["exports:src/a.ts:bar"]);
  assert.equal(res.ok, false);
  assert.equal(res.added.length, 1);
  assert.equal(res.removed.length, 1);
});

test("flatten keys files, exports, types and dependencies without line numbers", () => {
  const report = {
    files: ["src/dead.tsx"],
    issues: [
      {
        file: "src/a.ts",
        exports: [{ name: "foo", line: 3, col: 1 }],
        types: [{ name: "Bar", line: 9, col: 1 }],
      },
      { file: "package.json", devDependencies: [{ name: "left-pad", line: 7, col: 6 }] },
    ],
  };
  assert.deepEqual(flatten(report), [
    "devDependencies:package.json:left-pad",
    "exports:src/a.ts:foo",
    "files:src/dead.tsx:",
    "types:src/a.ts:Bar",
  ]);
});

test("flatten reads an unused file from the issue that knip's json reporter emits", () => {
  const report = { issues: [{ file: "src/dead.tsx", files: [{ name: "src/dead.tsx" }], exports: [] }] };
  assert.deepEqual(flatten(report), ["files:src/dead.tsx:"]);
});

test("flatten ignores style findings that are not dead code", () => {
  const report = {
    issues: [{ file: "src/a.ts", duplicates: [{ name: "x" }], enumMembers: [{ name: "Y" }] }],
  };
  assert.deepEqual(flatten(report), []);
});

test("flatten collapses an item reported twice and tolerates an empty report", () => {
  const report = {
    issues: [{ file: "src/a.ts", exports: [{ name: "foo" }, { name: "foo" }] }],
  };
  assert.deepEqual(flatten(report), ["exports:src/a.ts:foo"]);
  assert.deepEqual(flatten({}), []);
});

test("every dead-code issue type is covered", () => {
  for (const type of ["exports", "types", "dependencies", "devDependencies", "unlisted", "unresolved"]) {
    assert.ok(ITEM_TYPES.includes(type), `${type} must be gated`);
  }
});

test("the refresh command names the script", () => {
  assert.match(refreshCommand(), /knip-ratchet\.mjs --write/);
});

const TABLE = [
  "Function                    Count  Min     Max     Median  Sum     %",
  "getImportsAndExports          136  0.01    6.38    0.30   83.38  10%",
].join("\n");

test("the analysed file count is read from the performance table", () => {
  assert.equal(analysedFiles(TABLE), 136);
  assert.equal(analysedFiles(""), 0);
  assert.equal(analysedFiles("Total running time: 3ms"), 0);
});

test("a run that analysed fewer files than the floor is an error", () => {
  assert.equal(checkFloor(TABLE, 136), null);
  assert.match(checkFloor(TABLE, 137), /analysed 136 file\(s\), expected at least 137/);
  assert.match(checkFloor("", MIN_FILES), /nothing was measured/);
});

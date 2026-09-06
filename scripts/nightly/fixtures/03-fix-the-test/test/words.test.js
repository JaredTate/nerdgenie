import test from "node:test";
import assert from "node:assert/strict";
import { countWords } from "../words.js";

test("counts the words of a plain sentence", () => {
  assert.equal(countWords("the kettle is on"), 4);
});

test("a run of spaces still separates two words", () => {
  assert.equal(countWords("the   kettle is on"), 4);
});

test("an empty sentence has no words", () => {
  assert.equal(countWords(""), 0);
});

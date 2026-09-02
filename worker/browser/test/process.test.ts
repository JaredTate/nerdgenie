import { afterAll, beforeAll, describe, expect, it } from "vitest";
import fc from "fast-check";
import { spawn, type ChildProcess } from "node:child_process";
import { access, mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { startFixtureServer, type FixtureServer } from "./server.js";
import type { JsonRpcResponse } from "../src/types.js";

const BUILT = join(fileURLToPath(new URL("..", import.meta.url)), "dist", "main.js");

/** The worker as the Go side really runs it: another process, spoken to over pipes. */
class SpawnedWorker {
  private readonly child: ChildProcess;
  private readonly answers = new Map<number, (response: JsonRpcResponse) => void>();
  readonly standardOutput: string[] = [];
  readonly standardError: string[] = [];
  private pending = "";
  private nextId = 1;

  constructor(profile: string) {
    this.child = spawn(process.execPath, [BUILT, "--profile", profile, "--pacing", "fast"], {
      stdio: ["pipe", "pipe", "pipe"],
    });
    this.child.stdout?.setEncoding("utf8");
    this.child.stdout?.on("data", (piece: string) => this.take(piece));
    this.child.stderr?.setEncoding("utf8");
    this.child.stderr?.on("data", (piece: string) => {
      this.standardError.push(piece);
    });
  }

  private take(piece: string): void {
    this.pending += piece;
    for (;;) {
      const end = this.pending.indexOf("\n");
      if (end === -1) {
        return;
      }
      const line = this.pending.slice(0, end);
      this.pending = this.pending.slice(end + 1);
      this.standardOutput.push(line);
      const response = JSON.parse(line) as JsonRpcResponse;
      if (typeof response.id === "number") {
        this.answers.get(response.id)?.(response);
        this.answers.delete(response.id);
      }
    }
  }

  /** Send one already-written line and do not wait for anything back. */
  writeRaw(line: string): void {
    this.child.stdin?.write(`${line}\n`);
  }

  /** Send one already-written line, whatever it holds, and wait for the next answer. */
  sendRaw(line: string): Promise<JsonRpcResponse> {
    const before = this.standardOutput.length;
    this.child.stdin?.write(`${line}\n`);
    return this.waitForLine(before);
  }

  send(method: string, params: Record<string, unknown> = {}): Promise<JsonRpcResponse> {
    const id = this.nextId;
    this.nextId += 1;
    const waiting = new Promise<JsonRpcResponse>((answered) => this.answers.set(id, answered));
    this.child.stdin?.write(`${JSON.stringify({ jsonrpc: "2.0", id, method, params })}\n`);
    return waiting;
  }

  private async waitForLine(before: number): Promise<JsonRpcResponse> {
    const giveUpAt = Date.now() + 20_000;
    while (this.standardOutput.length === before && Date.now() < giveUpAt) {
      await new Promise((next) => setTimeout(next, 20));
    }
    const line = this.standardOutput[before];
    if (line === undefined) {
      throw new Error("the worker answered nothing at all");
    }
    return JSON.parse(line) as JsonRpcResponse;
  }

  /** Wait until the worker has started Chrome and can answer. */
  async ready(): Promise<void> {
    const answer = await this.send("health");
    if (!("result" in answer)) {
      throw new Error(`the worker never became healthy: ${JSON.stringify(answer)}`);
    }
  }

  async stop(): Promise<void> {
    this.child.stdin?.end();
    const giveUpAt = Date.now() + 10_000;
    while (this.child.exitCode === null && this.child.signalCode === null && Date.now() < giveUpAt) {
      await new Promise((next) => setTimeout(next, 25));
    }
    if (this.child.exitCode === null && this.child.signalCode === null) {
      this.child.kill("SIGKILL");
    }
  }
}

describe("the worker as its own process", () => {
  let worker: SpawnedWorker;
  let site: FixtureServer;
  let profile: string;

  beforeAll(async () => {
    await access(BUILT).catch(() => {
      throw new Error(`${BUILT} is missing. Run npm run build before npm test.`);
    });
    site = await startFixtureServer();
    profile = await mkdtemp(join(tmpdir(), "coeus-browser-process-"));
    worker = new SpawnedWorker(profile);
    await worker.ready();
  });

  afterAll(async () => {
    await worker.stop();
    await site.stop();
    await rm(profile, { recursive: true, force: true });
  });

  it("writes nothing to standard output but responses, one per line", async () => {
    await worker.send("open", { url: site.page("links-and-form.html") });
    await worker.send("read");
    expect(worker.standardOutput.length).toBeGreaterThanOrEqual(3);
    for (const line of worker.standardOutput) {
      const parsed = JSON.parse(line) as JsonRpcResponse;
      expect(parsed.jsonrpc).toBe("2.0");
      expect("result" in parsed || "error" in parsed).toBe(true);
    }
  });

  it("puts its own logging on standard error, where it cannot break the framing", () => {
    expect(worker.standardError.join("")).toContain("browser worker: ");
  });

  it("answers a line that is not JSON with -32700 and keeps going", async () => {
    const answer = await worker.sendRaw("this is not JSON at all");
    expect("error" in answer && answer.error.code).toBe(-32700);
    const after = await worker.send("health");
    expect("result" in after).toBe(true);
  });

  it("ignores a blank line rather than treating it as a broken request", async () => {
    const before = worker.standardOutput.length;
    worker.writeRaw("   ");
    const after = await worker.send("health");
    expect("result" in after).toBe(true);
    // One line came back, and it was the answer to health, not to the blank line.
    expect(worker.standardOutput.length).toBe(before + 1);
  });

  it("answers arbitrary bytes without ever falling over", async () => {
    await fc.assert(
      fc.asyncProperty(fc.uint8Array({ minLength: 1, maxLength: 120 }), async (bytes) => {
        const line = Buffer.from(bytes).toString("utf8").replace(/[\n\r]/g, " ");
        if (line.trim() === "") {
          return;
        }
        const answer = await worker.sendRaw(line);
        expect(answer.jsonrpc).toBe("2.0");
      }),
      { numRuns: 30 },
    );
    const after = await worker.send("health");
    expect("result" in after).toBe(true);
  });

  it("answers any sequence of refs at all, and never throws", async () => {
    await worker.send("open", { url: site.page("links-and-form.html") });
    await fc.assert(
      fc.asyncProperty(fc.array(fc.string({ maxLength: 12 }), { maxLength: 3 }), async (refs) => {
        for (const ref of refs) {
          const answer = await worker.send("click", { ref });
          expect("result" in answer || "error" in answer).toBe(true);
        }
      }),
      { numRuns: 4 },
    );
    const after = await worker.send("health");
    expect("result" in after).toBe(true);
  });

  it("stops when standard input closes, and takes Chrome with it", async () => {
    const alone = await mkdtemp(join(tmpdir(), "coeus-browser-exit-"));
    const other = new SpawnedWorker(alone);
    await other.ready();
    await other.stop();
    expect(other.standardError.join("")).toContain("stopped");
    await rm(alone, { recursive: true, force: true });
  });
});

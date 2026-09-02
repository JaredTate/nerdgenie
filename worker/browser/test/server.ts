/**
 * The tiny static server the browser tests run against.
 *
 * It serves worker/browser/test/pages on a loopback port the operating system
 * picks, so no test ever has to reserve a port and two test files can never
 * collide. It serves nothing outside that folder.
 */
import { createServer, type Server } from "node:http";
import { readFile } from "node:fs/promises";
import { extname, join, normalize, sep } from "node:path";
import { fileURLToPath } from "node:url";

const PAGES = join(fileURLToPath(new URL(".", import.meta.url)), "pages");

const CONTENT_TYPES: Readonly<Record<string, string>> = {
  ".html": "text/html; charset=utf-8",
  ".txt": "text/plain; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
};

/** A running fixture server: where to reach it, and how to stop it. */
export interface FixtureServer {
  /** The address the pages are served from, ending in a slash. */
  base: string;
  /** The address of one page. */
  page(name: string): string;
  stop(): Promise<void>;
}

/** Turn a request path into a file inside the pages folder, or nothing. */
function fileFor(requestPath: string): string | undefined {
  const wanted = normalize(decodeURIComponent(requestPath.split("?")[0] ?? "/")).replace(
    /^(\.\.[/\\])+/,
    "",
  );
  const file = join(PAGES, wanted);
  return file.startsWith(PAGES + sep) ? file : undefined;
}

function serve(server: Server): void {
  server.on("request", (request, response) => {
    const file = fileFor(request.url ?? "/");
    if (file === undefined) {
      response.writeHead(403).end("Outside the pages folder.");
      return;
    }
    readFile(file).then(
      (body) => {
        const type = CONTENT_TYPES[extname(file)] ?? "application/octet-stream";
        response.writeHead(200, { "content-type": type, "cache-control": "no-store" }).end(body);
      },
      () => {
        response.writeHead(404).end("No such page.");
      },
    );
  });
}

/** Start the fixture server on a loopback port the operating system picks. */
export async function startFixtureServer(): Promise<FixtureServer> {
  const server = createServer();
  serve(server);
  await new Promise<void>((listening) => server.listen(0, "127.0.0.1", listening));
  const address = server.address();
  if (address === null || typeof address === "string") {
    throw new Error("The fixture server did not report a port to listen on.");
  }
  const base = `http://127.0.0.1:${address.port}/`;
  return {
    base,
    page: (name: string) => `${base}${name}`,
    stop: () =>
      new Promise<void>((stopped, failed) => {
        server.close((problem) => (problem ? failed(problem) : stopped()));
      }),
  };
}

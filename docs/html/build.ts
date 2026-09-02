import { marked, Renderer } from "marked";
import { readFileSync, writeFileSync } from "fs";

const [, , inPath, outPath, title, favTitle] = process.argv;
const md = readFileSync(inPath, "utf8");

const r = new Renderer();
const esc = (s: string) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");

r.code = ({ text, lang }: { text: string; lang?: string }) => {
  if (lang === "mermaid") return `<div class="diagram"><pre class="mermaid">\n${text}\n</pre></div>\n`;
  return `<div class="codewrap"><pre><code>${esc(text)}</code></pre></div>\n`;
};
r.table = (token: any) => {
  const head = `<tr>${token.header.map((c: any) => `<th>${marked.parseInline(c.text)}</th>`).join("")}</tr>`;
  const body = token.rows
    .map((row: any[]) => `<tr>${row.map((c: any) => `<td>${marked.parseInline(c.text)}</td>`).join("")}</tr>`)
    .join("\n");
  return `<div class="tablewrap"><table><thead>${head}</thead><tbody>${body}</tbody></table></div>\n`;
};
r.heading = ({ text, depth, tokens }: any) => {
  const inner = marked.parseInline(text);
  const id = text.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
  return `<h${depth} id="${id}">${inner}</h${depth}>\n`;
};
marked.use({ renderer: r, gfm: true });

let body = marked.parse(md) as string;
// Drop the duplicated first h1 (file name) in favour of the page header.
body = body.replace(/^<h1 id="[^"]*">[^<]*<\/h1>\n/, "");
// Colour the verdict language.
body = body.replace(/<strong>Not for you<\/strong>/g, '<strong class="bad">Not for you</strong>');

// Build a nav from Part headings.
let parts = [...body.matchAll(/<h1 id="([^"]+)">(.*?)<\/h1>/g)].map((m) => ({ id: m[1], t: m[2].replace(/<[^>]+>/g, "") }));
if (parts.length === 0) parts = [...body.matchAll(/<h2 id="([^"]+)">(.*?)<\/h2>/g)].map((m) => ({ id: m[1], t: m[2].replace(/<[^>]+>/g, "").replace(/^\d+\.\s*/, "") }));
const nav = parts.map((p) => `<a href="#${p.id}">${p.t.replace(/^Part \d+ — /, "").replace(/^Appendix — /, "")}</a>`).join("");

const html = `<title>${title}</title>
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Bricolage+Grotesque:opsz,wght@12..96,500;12..96,700&family=Source+Sans+3:ital,wght@0,400;0,600;1,400&family=IBM+Plex+Mono:wght@400;500&display=swap">
<style>
:root{
  --bg:#F7F8F6; --bg2:#EEF1ED; --ink:#1D2320; --ink2:#4A5450; --line:#D6DBD6;
  --accent:#0F6E6E; --accent-ink:#0A5252; --bad:#A64B2A; --good:#2E7D4F; --quote:#E4EFEE;
  --display:"Bricolage Grotesque","Avenir Next",Helvetica,Arial,sans-serif;
  --body:"Source Sans 3","Source Sans Pro",Helvetica,Arial,sans-serif;
  --mono:"IBM Plex Mono",Menlo,Consolas,monospace;
}
@media (prefers-color-scheme: dark){ :root:not([data-theme="light"]){
  --bg:#141816; --bg2:#1C221F; --ink:#E6E9E4; --ink2:#A7B0AA; --line:#2E3733;
  --accent:#5CC5C0; --accent-ink:#8ADAD6; --bad:#E08A6B; --good:#7CC59A; --quote:#1B2A29;
}}
:root[data-theme="dark"]{
  --bg:#141816; --bg2:#1C221F; --ink:#E6E9E4; --ink2:#A7B0AA; --line:#2E3733;
  --accent:#5CC5C0; --accent-ink:#8ADAD6; --bad:#E08A6B; --good:#7CC59A; --quote:#1B2A29;
}
html{color-scheme:light dark}
body{background:var(--bg);color:var(--ink);font-family:var(--body);font-size:17px;line-height:1.55;margin:0}
.page{max-width:1120px;margin:0 auto;padding:0 24px 96px}
header.top{padding:48px 0 20px;border-bottom:1px solid var(--line);margin-bottom:8px}
header.top h1{font-family:var(--display);font-weight:700;font-size:clamp(28px,4vw,44px);line-height:1.05;margin:0 0 10px;text-wrap:balance;letter-spacing:-0.01em}
header.top p{margin:0;color:var(--ink2);max-width:70ch}
nav.parts{position:sticky;top:0;background:var(--bg);border-bottom:1px solid var(--line);display:flex;gap:4px 18px;flex-wrap:wrap;padding:10px 0;font-family:var(--mono);font-size:12.5px;letter-spacing:.02em;z-index:2}
nav.parts a{color:var(--accent-ink);text-decoration:none;padding:2px 0}
nav.parts a:hover,nav.parts a:focus-visible{text-decoration:underline;outline:none}
main{display:grid;grid-template-columns:1fr;gap:0}
main>p,main>ul,main>ol,main>blockquote,main>h1,main>h2,main>h3{max-width:68ch}
h1{font-family:var(--display);font-weight:700;font-size:clamp(24px,3vw,34px);line-height:1.1;margin:64px 0 12px;padding-top:24px;border-top:3px solid var(--accent);text-wrap:balance;letter-spacing:-0.01em}
h2{font-family:var(--display);font-weight:700;font-size:23px;line-height:1.15;margin:40px 0 10px;text-wrap:balance}
h3{font-family:var(--display);font-weight:500;font-size:18px;margin:28px 0 8px;color:var(--ink2);text-transform:uppercase;letter-spacing:.06em}
p{margin:0 0 14px}
ul,ol{margin:0 0 14px;padding-left:1.3em}
li{margin:0 0 6px}
li>p{margin:0}
a{color:var(--accent-ink)}
strong{font-weight:600}
strong.bad{color:var(--bad)}
code{font-family:var(--mono);font-size:.88em;background:var(--bg2);padding:1px 5px;border-radius:3px}
blockquote{margin:18px 0 22px;padding:14px 18px;background:var(--quote);border-left:3px solid var(--accent);font-family:var(--display);font-weight:500;font-size:19px;line-height:1.35;text-wrap:balance}
blockquote p{margin:0}
hr{border:0;border-top:1px solid var(--line);margin:32px 0}
.tablewrap{overflow-x:auto;margin:14px 0 26px;border:1px solid var(--line);border-radius:4px}
table{border-collapse:collapse;width:100%;font-size:14.5px;font-variant-numeric:tabular-nums}
th,td{padding:9px 12px;text-align:left;vertical-align:top;border-bottom:1px solid var(--line)}
th{font-family:var(--mono);font-size:12px;letter-spacing:.04em;text-transform:uppercase;color:var(--ink2);background:var(--bg2);white-space:nowrap}
td:first-child{font-weight:600;white-space:nowrap}
tbody tr:last-child td{border-bottom:0}
td code,th code{white-space:nowrap}
.diagram{margin:18px 0 26px;padding:18px 12px;background:var(--bg2);border:1px solid var(--line);border-radius:4px;overflow-x:auto}
.diagram pre.mermaid{margin:0;background:transparent;font-family:var(--mono);font-size:13px}
.codewrap{overflow-x:auto;margin:14px 0 22px;background:var(--bg2);border:1px solid var(--line);border-radius:4px}
.codewrap pre{margin:0;padding:14px 16px;font-family:var(--mono);font-size:13.5px;line-height:1.5}
.codewrap code{background:transparent;padding:0;font-size:inherit}
@media (prefers-reduced-motion: reduce){ *{scroll-behavior:auto!important} }
html{scroll-behavior:smooth}
</style>
<div class="page">
<header class="top">
  <h1>${title}</h1>
  <p>${parts.length} parts. Diagrams draw below; tables scroll sideways on a phone. Source: <code>docs/${inPath.split("/").pop()}</code> in the Coeus repository.</p>
</header>
<nav class="parts">${nav}</nav>
<main>
${body}
</main>
</div>
`;
writeFileSync(outPath, html);
console.log(outPath, html.length, "bytes,", parts.length, "parts,", (html.match(/class="mermaid"/g) || []).length, "diagrams");

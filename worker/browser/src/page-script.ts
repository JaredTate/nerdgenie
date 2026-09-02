/**
 * The JavaScript that runs inside the page.
 *
 * It is kept as text rather than as TypeScript functions for two reasons. It runs
 * in Chrome and not in Node, so it may only use what a page has and nothing this
 * program imports; and it is sent to the page whole, so it must be self-contained.
 *
 * Every piece attaches itself to `window` behind an `||`, so sending it twice into
 * the same page costs nothing and never redeclares anything. That idempotent
 * install is borrowed from Moltis's snapshot helpers at
 * docs/reference/moltis/snapshot.rs, as is walking into open shadow roots. What
 * elements to keep, and marking the ones a person would have to scroll to see, is
 * borrowed from browser-use's serializer at
 * docs/reference/browser-use/serializer.py. The JavaScript is written fresh.
 *
 * One difference from both of those is deliberate. Moltis wipes every ref and
 * renumbers from one on each snapshot, which means a ref means something different
 * a second later. Here a ref is written onto the element as an attribute and never
 * reused, so it names the same element for as long as that element is on the page,
 * which is what PROTOCOL.md promises the model.
 */

/** Walk the elements of a document, including into open shadow roots. */
const WALK = `
window.__coeusWalk = window.__coeusWalk || function (root, mostNodes, visit) {
  var waiting = [root];
  var seen = 0;
  while (waiting.length > 0 && seen < mostNodes) {
    var node = waiting.pop();
    if (node.shadowRoot) { waiting.push(node.shadowRoot); }
    var children = node.children || [];
    for (var i = children.length - 1; i >= 0; i -= 1) { waiting.push(children[i]); }
    if (node.nodeType === 1) { seen += 1; visit(node); }
  }
  return seen;
};
`;

/** What kind of thing an element is. An explicit role attribute always wins. */
const ROLE = `
window.__coeusRoleOf = window.__coeusRoleOf || function (element) {
  var stated = (element.getAttribute("role") || "").trim().split(/\\s+/)[0].toLowerCase();
  if (stated) { return stated; }
  var tag = element.tagName.toLowerCase();
  if (tag === "a") { return element.hasAttribute("href") ? "link" : ""; }
  if (tag === "button" || tag === "summary") { return "button"; }
  if (tag === "form") { return "form"; }
  if (tag === "article") { return "article"; }
  if (tag === "textarea") { return "textbox"; }
  if (tag === "select") { return "combobox"; }
  if (/^h[1-6]$/.test(tag)) { return "heading"; }
  if (tag === "input") {
    var kind = (element.getAttribute("type") || "text").toLowerCase();
    if (kind === "checkbox") { return "checkbox"; }
    if (kind === "radio") { return "radio"; }
    if (kind === "button" || kind === "submit" || kind === "reset" || kind === "image") { return "button"; }
    if (kind === "hidden") { return ""; }
    return "textbox";
  }
  if (element.isContentEditable) { return "textbox"; }
  return "";
};
`;

/**
 * What an element is called. The order is the one a screen reader uses. A form
 * never falls back to its own text, because a form's text is the whole page.
 * A password field's value is never read at all.
 */
const NAME = `
window.__coeusTidy = window.__coeusTidy || function (text) {
  return (text || "").replace(/\\s+/g, " ").trim();
};
window.__coeusLabelledBy = window.__coeusLabelledBy || function (element) {
  var ids = (element.getAttribute("aria-labelledby") || "").trim();
  if (!ids) { return ""; }
  var parts = [];
  ids.split(/\\s+/).forEach(function (id) {
    var other = document.getElementById(id);
    if (other) { parts.push(window.__coeusTidy(other.textContent)); }
  });
  return window.__coeusTidy(parts.join(" "));
};
window.__coeusNameOf = window.__coeusNameOf || function (element, role, mostCharacters) {
  var tidy = window.__coeusTidy;
  var name = tidy(element.getAttribute("aria-label")) || window.__coeusLabelledBy(element);
  if (!name && element.id) {
    var tied = document.querySelector('label[for="' + window.CSS.escape(element.id) + '"]');
    if (tied) { name = tidy(tied.textContent); }
  }
  if (!name && element.closest) {
    var wrapping = element.closest("label");
    if (wrapping) { name = tidy(wrapping.textContent); }
  }
  if (!name) { name = tidy(element.getAttribute("placeholder")); }
  if (!name) { name = tidy(element.getAttribute("alt")); }
  if (!name) { name = tidy(element.getAttribute("title")); }
  if (!name && element.tagName === "INPUT" && /^(submit|button|reset)$/i.test(element.type)) {
    name = tidy(element.value);
  }
  if (!name && role === "form") {
    var inside = element.querySelector("legend, h1, h2, h3, h4, h5, h6");
    if (inside) { name = tidy(inside.textContent); }
  }
  if (!name && role !== "form") { name = tidy(element.innerText || element.textContent); }
  if (name.length > mostCharacters) { name = name.slice(0, mostCharacters - 1) + "\\u2026"; }
  return name;
};
`;

/** Whether an element is drawn at all, and whether it takes a short run of digits. */
const SHAPE = `
window.__coeusRendered = window.__coeusRendered || function (box, style) {
  return box.width > 0 && box.height > 0 &&
    style.visibility !== "hidden" && style.display !== "none" &&
    parseFloat(style.opacity || "1") > 0;
};
window.__coeusShortNumeric = window.__coeusShortNumeric || function (element) {
  if (element.tagName !== "INPUT") { return false; }
  var kind = (element.getAttribute("type") || "text").toLowerCase();
  if (["text", "tel", "number", "password"].indexOf(kind) === -1) { return false; }
  var mostCharacters = parseInt(element.getAttribute("maxlength") || "0", 10);
  var mode = (element.getAttribute("inputmode") || "").toLowerCase();
  var shape = element.getAttribute("pattern") || "";
  return (mostCharacters > 0 && mostCharacters <= 8) || mode === "numeric" || /\\d/.test(shape);
};
`;

/**
 * Look at everything on this document and report it. Every element that is kept
 * gets a ref written onto it, and a ref is never handed out twice, so a ref names
 * the same element for as long as that element lives.
 */
const SCAN = `
window.__coeusScan = window.__coeusScan || function (how) {
  if (typeof window.__coeusNextRef !== "number") { window.__coeusNextRef = how.refBase; }
  var wide = window.innerWidth;
  var tall = window.innerHeight;
  var found = [];
  window.__coeusWalk(document, how.mostNodes, function (element) {
    var role = window.__coeusRoleOf(element);
    if (!role || how.roles.indexOf(role) === -1) { return; }
    var box = element.getBoundingClientRect();
    var style = window.getComputedStyle(element);
    if (!window.__coeusRendered(box, style)) { return; }
    var ref = element.getAttribute("data-coeus-ref");
    if (!ref) {
      ref = "e" + window.__coeusNextRef;
      window.__coeusNextRef += 1;
      element.setAttribute("data-coeus-ref", ref);
    }
    found.push({
      ref: ref,
      role: role,
      name: window.__coeusNameOf(element, role, how.mostNameCharacters),
      aboveFold: box.bottom > 0 && box.right > 0 && box.top < tall && box.left < wide,
      password: element.tagName === "INPUT" && (element.getAttribute("type") || "").toLowerCase() === "password",
      shortNumeric: window.__coeusShortNumeric(element)
    });
  });
  return { url: location.href, title: document.title, contentType: document.contentType, elements: found };
};
`;

/** Find an element again after its ref went stale, and give the answer a fresh ref. */
const FIND = `
window.__coeusStamp = window.__coeusStamp || function (element, refBase) {
  if (typeof window.__coeusNextRef !== "number") { window.__coeusNextRef = refBase; }
  var ref = element.getAttribute("data-coeus-ref");
  if (!ref) {
    ref = "e" + window.__coeusNextRef;
    window.__coeusNextRef += 1;
    element.setAttribute("data-coeus-ref", ref);
  }
  return ref;
};
window.__coeusFindLike = window.__coeusFindLike || function (how) {
  var wanted = (how.name || "").toLowerCase();
  var match = null;
  window.__coeusWalk(document, how.mostNodes, function (element) {
    if (match) { return; }
    var role = window.__coeusRoleOf(element);
    if (!role) { return; }
    if (how.role && role !== how.role) { return; }
    var box = element.getBoundingClientRect();
    var style = window.getComputedStyle(element);
    if (!window.__coeusRendered(box, style)) { return; }
    var name = window.__coeusNameOf(element, role, how.mostNameCharacters).toLowerCase();
    var text = window.__coeusTidy(element.innerText || element.textContent).toLowerCase();
    var hit = how.byText ? (name.indexOf(wanted) !== -1 || text.indexOf(wanted) !== -1) : name === wanted;
    if (hit && wanted) { match = element; }
  });
  return match ? window.__coeusStamp(match, how.refBase) : null;
};
`;

/** Where an element is on the screen, so a click can be retried at its place. */
const BOX = `
window.__coeusBoxOf = window.__coeusBoxOf || function (ref) {
  var element = document.querySelector('[data-coeus-ref="' + ref + '"]');
  if (!element) { return null; }
  var box = element.getBoundingClientRect();
  return { x: box.x, y: box.y, width: box.width, height: box.height };
};
`;

/**
 * Watching for changes, which is how the worker knows the page has gone quiet.
 * It installs itself once per document and starts again after every move to a new
 * address, because the new document is a new window.
 *
 * Attributes are deliberately not watched. A spinner, a progress bar, and a class
 * swapped by an animation all change nothing but attributes, over and over, and
 * none of them means the page is still doing something worth waiting for.
 */
const WATCH = `
window.__coeusInstallWatch = window.__coeusInstallWatch || function () {
  if (window.__coeusWatching) { return; }
  var root = document.documentElement;
  if (!root) {
    document.addEventListener("DOMContentLoaded", window.__coeusInstallWatch);
    return;
  }
  window.__coeusWatching = true;
  window.__coeusLastChange = Date.now();
  new window.MutationObserver(function () { window.__coeusLastChange = Date.now(); })
    .observe(root, { subtree: true, childList: true, characterData: true });
};
window.__coeusQuietFor = window.__coeusQuietFor || function () {
  window.__coeusInstallWatch();
  return typeof window.__coeusLastChange === "number" ? Date.now() - window.__coeusLastChange : 0;
};
window.__coeusInstallWatch();
`;

/** Draw a numbered mark on each element a screenshot should point at, then take them away. */
const MARKS = `
window.__coeusDrawMarks = window.__coeusDrawMarks || function (marks) {
  window.__coeusClearMarks();
  var sheet = document.createElement("div");
  sheet.id = "coeus-marks";
  sheet.style.cssText = "position:fixed;left:0;top:0;width:0;height:0;z-index:2147483647;pointer-events:none";
  marks.forEach(function (mark) {
    var element = document.querySelector('[data-coeus-ref="' + mark.ref + '"]');
    if (!element) { return; }
    var box = element.getBoundingClientRect();
    var tag = document.createElement("div");
    tag.textContent = String(mark.number);
    tag.style.cssText = "position:fixed;left:" + Math.max(0, box.x) + "px;top:" + Math.max(0, box.y) +
      "px;background:#c1121f;color:#fff;font:bold 12px sans-serif;padding:1px 4px;border-radius:3px";
    sheet.appendChild(tag);
    var edge = document.createElement("div");
    edge.style.cssText = "position:fixed;left:" + box.x + "px;top:" + box.y + "px;width:" + box.width +
      "px;height:" + box.height + "px;outline:2px solid #c1121f";
    sheet.appendChild(edge);
  });
  document.body.appendChild(sheet);
  return marks.length;
};
window.__coeusClearMarks = window.__coeusClearMarks || function () {
  var sheet = document.getElementById("coeus-marks");
  if (sheet && sheet.parentNode) { sheet.parentNode.removeChild(sheet); }
  return true;
};
`;

/** Everything above, in the order it depends on itself. */
export const PAGE_SCRIPT = [WALK, ROLE, NAME, SHAPE, SCAN, FIND, BOX, WATCH, MARKS].join("\n");

/** Wrap a call to one of the page's own functions so it can be sent on its own. */
export function pageCall(expression: string): string {
  return `(function () {\n${PAGE_SCRIPT}\nreturn ${expression};\n})()`;
}

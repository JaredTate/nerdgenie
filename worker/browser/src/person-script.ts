/**
 * The JavaScript that watches what the person does, running inside the page.
 *
 * It is kept as text for the same two reasons page-script.ts is: it runs in
 * Chrome and not in Node, and it is sent to the page whole. It is installed at
 * the start of every document, so a person who clicks before the agent has read
 * the page is still seen.
 *
 * Nothing here reads what was typed. A typing event carries how many characters
 * the box holds and nothing else, so that a recording of a person signing in
 * cannot become a copy of their password.
 */
import { MAX_ELEMENT_NAME_CHARS, PERSON_TYPING_QUIET_MS } from "./limits.js";
import { PAGE_SCRIPT } from "./page-script.js";

/** The name the page calls to say what the person just did. */
export const PERSON_BINDING = "__nerdgeniePersonDid";

/** How far up from the clicked node the watcher looks for something worth naming. */
const MOST_STEPS_UP = 5;

/**
 * The ref number a watcher mints from when nothing has scanned the document yet.
 * It is the same base the main frame's own scan starts at, so a ref minted here
 * and a ref minted by a snapshot never mean two different elements.
 */
const REF_BASE = 1;

const WATCH = `
window.__nerdgenieWatchPerson = window.__nerdgenieWatchPerson || function () {
  if (window.__nerdgenieWatchingPerson) { return false; }
  if (window.top !== window) { return false; }
  window.__nerdgenieWatchingPerson = true;
  var tell = function (what) {
    try {
      if (typeof window.${PERSON_BINDING} === "function") { window.${PERSON_BINDING}(what); }
    } catch (problem) { }
  };
  var worthNaming = function (node) {
    var element = node;
    for (var step = 0; element && step < ${MOST_STEPS_UP}; step += 1) {
      if (element.nodeType === 1) {
        if (element.getAttribute("data-nerdgenie-ref")) { return element; }
        if (window.__nerdgenieRoleOf(element)) { return element; }
      }
      element = element.parentElement;
    }
    return node && node.nodeType === 1 ? node : null;
  };
  document.addEventListener("click", function (happening) {
    var element = worthNaming(happening.target);
    if (!element) { return; }
    var role = window.__nerdgenieRoleOf(element);
    tell({
      kind: "click",
      ref: window.__nerdgenieStamp(element, ${REF_BASE}),
      text: window.__nerdgenieNameOf(element, role, ${MAX_ELEMENT_NAME_CHARS}),
      startedAt: Date.now()
    });
  }, true);

  var typingIn = null;
  var typingTimer = null;
  var typingStartedAt = 0;
  var tellAboutTyping = function () {
    if (typingTimer) { window.clearTimeout(typingTimer); typingTimer = null; }
    var box = typingIn;
    typingIn = null;
    if (!box) { return; }
    var held = typeof box.value === "string" ? box.value : (box.textContent || "");
    tell({
      kind: "type",
      ref: window.__nerdgenieStamp(box, ${REF_BASE}),
      length: held.length,
      startedAt: typingStartedAt
    });
  };
  document.addEventListener("input", function (happening) {
    var box = happening.target;
    if (!box || box.nodeType !== 1) { return; }
    if (typingIn && typingIn !== box) { tellAboutTyping(); }
    if (!typingIn) { typingStartedAt = Date.now(); }
    typingIn = box;
    if (typingTimer) { window.clearTimeout(typingTimer); }
    typingTimer = window.setTimeout(tellAboutTyping, ${PERSON_TYPING_QUIET_MS});
  }, true);
  document.addEventListener("change", function () { tellAboutTyping(); }, true);
  document.addEventListener("blur", function () { tellAboutTyping(); }, true);
  return true;
};
window.__nerdgenieWatchPerson();
`;

/**
 * Everything the watcher needs: the page's own helpers, so that an element can be
 * given a ref and read the way a snapshot reads it, and the watcher itself.
 */
export const PERSON_SCRIPT = [PAGE_SCRIPT, WATCH].join("\n");

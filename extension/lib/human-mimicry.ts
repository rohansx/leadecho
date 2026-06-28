/**
 * Human-mimicry typing engine.
 *
 * Simulates realistic human keystrokes with Gaussian timing and punctuation
 * pauses, while injecting text in a way that React- and Draft.js-controlled
 * editors actually register (so the host page's submit button becomes enabled).
 *
 * `simulateTyping` returns a boolean indicating whether the text actually
 * landed in the target element — callers MUST use this to avoid reporting a
 * reply as posted when injection silently failed.
 */

/** Box-Muller transform: returns a normally-distributed random value. */
function gaussianRandom(mean: number, stdDev: number): number {
  let u = 0,
    v = 0;
  while (u === 0) u = Math.random();
  while (v === 0) v = Math.random();
  const n = Math.sqrt(-2.0 * Math.log(u)) * Math.cos(2.0 * Math.PI * v);
  return Math.max(20, mean + n * stdDev);
}

function randomBetween(min: number, max: number): number {
  return Math.floor(Math.random() * (max - min + 1)) + min;
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function isTextField(el: Element): el is HTMLInputElement | HTMLTextAreaElement {
  return el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement;
}

/** Reads the current text from either a form field or a contentEditable node. */
function readText(el: HTMLElement): string {
  if (isTextField(el)) return el.value;
  return el.textContent ?? "";
}

/**
 * Sets a form field's value through the native prototype setter so React's
 * internal value tracker is bypassed and the subsequent `input` event triggers
 * the component's onChange (the well-known controlled-input automation trick).
 */
function setFieldValue(el: HTMLInputElement | HTMLTextAreaElement, value: string): void {
  const proto =
    el instanceof HTMLTextAreaElement
      ? HTMLTextAreaElement.prototype
      : HTMLInputElement.prototype;
  const setter = Object.getOwnPropertyDescriptor(proto, "value")?.set;
  if (setter) setter.call(el, value);
  else el.value = value;
}

function focusAndPlaceCaret(el: HTMLElement): void {
  el.focus();
  if (isTextField(el)) {
    const end = el.value.length;
    try {
      el.setSelectionRange(end, end);
    } catch {
      /* number/email inputs disallow setSelectionRange — ignore */
    }
    return;
  }
  const sel = window.getSelection();
  if (!sel) return;
  const range = document.createRange();
  range.selectNodeContents(el);
  range.collapse(false);
  sel.removeAllRanges();
  sel.addRange(range);
}

/** Inserts a single character at the caret, dispatching React-compatible events. */
function insertChar(el: HTMLElement, char: string): void {
  if (isTextField(el)) {
    setFieldValue(el, el.value + char);
    el.dispatchEvent(
      new InputEvent("input", { bubbles: true, data: char, inputType: "insertText" }),
    );
    return;
  }

  // contentEditable (Draft.js, Quill, ProseMirror, shreddit composer, …).
  // execCommand("insertText") is still the most broadly-compatible path for
  // these editors and fires native beforeinput/input events the editor needs.
  let inserted = false;
  try {
    inserted = document.execCommand("insertText", false, char);
  } catch {
    inserted = false;
  }

  if (!inserted) {
    // Manual fallback: splice the character in at the current selection and
    // notify the editor ourselves.
    const sel = window.getSelection();
    if (sel && sel.rangeCount > 0) {
      const range = sel.getRangeAt(0);
      range.deleteContents();
      const node = document.createTextNode(char);
      range.insertNode(node);
      range.setStartAfter(node);
      range.collapse(true);
      sel.removeAllRanges();
      sel.addRange(range);
    } else {
      el.append(char);
    }
    el.dispatchEvent(
      new InputEvent("input", { bubbles: true, data: char, inputType: "insertText" }),
    );
  }
}

/**
 * Types `text` into a form field or contentEditable element with human-like
 * timing. A pre-engagement delay of 2–8 seconds simulates reading before
 * replying.
 *
 * @returns true if the element's text grew (injection succeeded), false otherwise.
 */
export async function simulateTyping(element: HTMLElement, text: string): Promise<boolean> {
  // Simulate reading + thinking before engaging.
  await sleep(randomBetween(2000, 8000));

  focusAndPlaceCaret(element);
  const before = readText(element).length;

  for (const char of text) {
    insertChar(element, char);

    let delay = gaussianRandom(80, 40);
    if (char === ".") delay += 300;
    else if (char === ",") delay += 150;
    else if (char === "\n") delay += 500;

    await sleep(delay);
  }

  // Fire a final change event for editors that commit on blur/change.
  element.dispatchEvent(new Event("change", { bubbles: true }));

  return readText(element).length > before;
}

/**
 * Human-mimicry typing engine.
 * Simulates realistic human keystrokes with Gaussian timing and punctuation pauses.
 */

/** Box-Muller transform: returns a normally-distributed random value. */
function gaussianRandom(mean: number, stdDev: number): number {
  let u = 0, v = 0;
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

/**
 * Types text into a contentEditable element with human-like timing.
 * Pre-engagement delay of 2–8 seconds simulates reading before replying.
 */
export async function simulateTyping(element: HTMLElement, text: string): Promise<void> {
  // Simulate reading + thinking before engaging
  await sleep(randomBetween(2000, 8000));

  element.focus();

  for (const char of text) {
    // Insert character using execCommand (works for contentEditable)
    document.execCommand("insertText", false, char);

    // Per-character delay: Gaussian ~80ms ± 40ms
    let delay = gaussianRandom(80, 40);

    // Extra pauses after punctuation (realistic typing rhythm)
    if (char === ".") delay += 300;
    else if (char === ",") delay += 150;
    else if (char === "\n") delay += 500;

    await sleep(delay);
  }
}

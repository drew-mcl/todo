// Normal mode for the capture box.
//
// A pure function of the text, where the caret is and what was pressed, so the
// whole thing can be tested without a browser. The component's only job is to
// hand it a keystroke and put back what comes out.
//
// What the keys are lives in internal/vim and is served to both front ends; the
// test here asserts that every one of them is answered.

export type Mode = "insert" | "normal";

export type State = {
  value: string;
  at: number;
  mode: Mode;
  /** A half-typed operator or prefix: the d of dd, the g of gg, the Z of ZZ. */
  pending: string;
};

/** What the box should do besides edit itself. */
export type Exit = "file" | "scrap" | "help";

export type Result = State & {
  /** False when the key was not ours and the textarea should have it. */
  handled: boolean;
  exit?: Exit;
};

const WORD = /[A-Za-z0-9_]/;

/** The bounds of the line the caret sits on, and where it sits within it. */
function line(value: string, at: number) {
  const start = value.lastIndexOf("\n", at - 1) + 1;
  const end = value.indexOf("\n", at);
  return { start, end: end === -1 ? value.length : end, column: at - start };
}

function firstThing(value: string, start: number, end: number) {
  let i = start;
  while (i < end && (value[i] === " " || value[i] === "\t")) i++;
  return i;
}

/** Where the next word starts, the way w does: over this word, then the gap. */
function nextWord(value: string, at: number) {
  let i = at;
  const inWord = i < value.length && WORD.test(value[i]);
  if (inWord) while (i < value.length && WORD.test(value[i])) i++;
  else if (i < value.length && !/\s/.test(value[i])) i++;
  while (i < value.length && /\s/.test(value[i])) i++;
  return i;
}

function prevWord(value: string, at: number) {
  let i = at - 1;
  while (i > 0 && /\s/.test(value[i])) i--;
  while (i > 0 && WORD.test(value[i - 1])) i--;
  return Math.max(0, i);
}

/** The end of this word, which is where e lands. */
function wordEnd(value: string, at: number) {
  let i = at + 1;
  while (i < value.length && /\s/.test(value[i])) i++;
  while (i + 1 < value.length && WORD.test(value[i + 1])) i++;
  return Math.min(i, Math.max(0, value.length - 1));
}

/** Up or down a line, holding the column as vim does. */
function vertical(value: string, at: number, by: number) {
  const here = line(value, at);
  if (by < 0) {
    if (here.start === 0) return at;
    const above = line(value, here.start - 1);
    return Math.min(above.start + here.column, above.end);
  }
  if (here.end >= value.length) return at;
  const below = line(value, here.end + 1);
  return Math.min(below.start + here.column, below.end);
}

function cut(s: State, from: number, to: number): Result {
  return {
    ...s,
    value: s.value.slice(0, from) + s.value.slice(to),
    at: from,
    pending: "",
    handled: true,
  };
}

function move(s: State, at: number): Result {
  return { ...s, at: Math.max(0, Math.min(at, s.value.length)), pending: "", handled: true };
}

function typing(s: State, at: number): Result {
  return { ...move(s, at), mode: "insert" };
}

/**
 * press applies one keystroke. Keys that are not ours come back unhandled, so
 * the textarea does what it would have done.
 */
export function press(key: string, s: State): Result {
  const kept: Result = { ...s, handled: false };
  const here = line(s.value, s.at);

  if (s.mode === "insert") {
    // The one key that means anything while typing. Everything else is text.
    if (key === "Escape") return { ...s, mode: "normal", pending: "", handled: true };
    return kept;
  }

  // A prefix has to be remembered between keystrokes.
  if (s.pending) {
    const had = s.pending;
    const cleared = { ...s, pending: "" };
    if (had === "d" && key === "d") return cut(cleared, here.start, Math.min(here.end + 1, s.value.length));
    if (had === "c" && key === "c") return { ...cut(cleared, here.start, here.end), mode: "insert" };
    if (had === "g" && key === "g") return move(cleared, 0);
    if (had === "Z" && key === "Z") return { ...cleared, handled: true, exit: "file" };
    if (had === "Z" && key === "Q") return { ...cleared, handled: true, exit: "scrap" };
    return { ...cleared, handled: true }; // an abandoned prefix eats its key
  }

  switch (key) {
    // Leaving.
    case "Escape":
      return { ...s, handled: true, exit: "file" };
    case "?":
      return { ...s, handled: true, exit: "help" };

    // Back to typing.
    case "i":
      return typing(s, s.at);
    case "a":
      return typing(s, Math.min(s.at + 1, here.end));
    case "I":
      return typing(s, firstThing(s.value, here.start, here.end));
    case "A":
      return typing(s, here.end);
    case "o":
      return {
        ...s,
        value: s.value.slice(0, here.end) + "\n" + s.value.slice(here.end),
        at: here.end + 1,
        mode: "insert",
        pending: "",
        handled: true,
      };
    case "O":
      return {
        ...s,
        value: s.value.slice(0, here.start) + "\n" + s.value.slice(here.start),
        at: here.start,
        mode: "insert",
        pending: "",
        handled: true,
      };

    // Moving.
    case "h":
      return move(s, Math.max(here.start, s.at - 1));
    case "l":
      return move(s, Math.min(here.end, s.at + 1));
    case "j":
      return move(s, vertical(s.value, s.at, 1));
    case "k":
      return move(s, vertical(s.value, s.at, -1));
    case "0":
      return move(s, here.start);
    case "^":
      return move(s, firstThing(s.value, here.start, here.end));
    case "$":
      return move(s, here.end);
    case "G":
      return move(s, s.value.length);

    // Words.
    case "w":
      return move(s, nextWord(s.value, s.at));
    case "b":
      return move(s, prevWord(s.value, s.at));
    case "e":
      return move(s, wordEnd(s.value, s.at));

    // Changing.
    case "x":
      return s.at < here.end ? cut(s, s.at, s.at + 1) : { ...s, handled: true };
    case "D":
      return cut(s, s.at, here.end);

    // Prefixes.
    case "d":
    case "c":
    case "g":
    case "Z":
      return { ...s, pending: key, handled: true };
  }

  // Undo is the browser's own, and every other key is swallowed: a stray letter
  // in normal mode must never end up in the draft.
  if (key === "u") return { ...s, handled: false };
  return { ...s, handled: true };
}

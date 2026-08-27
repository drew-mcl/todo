import { describe, expect, it } from "vitest";
import { press, type Mode, type State } from "../lib/vim";

/** Drive a sequence of keys over a draft. `|` marks the caret. */
function run(start: string, keys: string[], mode: Mode = "normal") {
  const at = start.indexOf("|");
  let s: State = { value: start.replace("|", ""), at: at < 0 ? 0 : at, mode, pending: "" };
  let exit: string | undefined;
  for (const key of keys) {
    const out = press(key, s);
    s = { value: out.value, at: out.at, mode: out.mode, pending: out.pending };
    if (out.exit) exit = out.exit;
  }
  return { ...s, exit, shown: s.value.slice(0, s.at) + "|" + s.value.slice(s.at) };
}

const draft = `prod issue | chase the vendor | today
admin | pull the numbers
personal | book the dentist`;

describe("moving", () => {
  it("goes left and right, and stops at the ends of the line", () => {
    expect(run("ab|c", ["h"]).shown).toBe("a|bc");
    expect(run("|abc", ["h"]).shown).toBe("|abc");
    expect(run("ab|c", ["l"]).shown).toBe("abc|");
    expect(run("abc|", ["l"]).shown).toBe("abc|");
  });

  it("goes up and down, holding the column", () => {
    expect(run("hello\nwo|rld", ["k"]).shown).toBe("he|llo\nworld");
    expect(run("he|llo\nworld", ["j"]).shown).toBe("hello\nwo|rld");
  });

  it("does not fall off a short line", () => {
    expect(run("ab\nlonger li|ne", ["k"]).shown).toBe("ab|\nlonger line");
  });

  it("reaches the ends of the line and the draft", () => {
    expect(run("  ab|c", ["0"]).shown).toBe("|  abc");
    expect(run("  ab|c", ["^"]).shown).toBe("  |abc");
    expect(run("a|bc", ["$"]).shown).toBe("abc|");
    expect(run(draft.replace("prod", "|prod"), ["G"]).at).toBe(draft.length);
    expect(run(`a\nb\nc|`, ["g", "g"]).at).toBe(0);
  });
});

describe("words", () => {
  it("jumps forward over a word and the gap after it", () => {
    expect(run("|chase the vendor", ["w"]).shown).toBe("chase |the vendor");
    expect(run("|chase the vendor", ["w", "w"]).shown).toBe("chase the |vendor");
  });

  it("jumps back", () => {
    expect(run("chase the ven|dor", ["b"]).shown).toBe("chase the |vendor");
    expect(run("chase the |vendor", ["b"]).shown).toBe("chase |the vendor");
  });

  it("reaches the end of a word", () => {
    expect(run("|chase the vendor", ["e"]).shown).toBe("chas|e the vendor");
  });
});

describe("changing", () => {
  it("deletes a character, but not the newline after the line", () => {
    expect(run("ab|c", ["x"]).value).toBe("ab");
    expect(run("abc|", ["x"]).value).toBe("abc");
  });

  it("deletes a whole line with dd, and takes its newline with it", () => {
    expect(run(`one\nt|wo\nthree`, ["d", "d"]).value).toBe("one\nthree");
    expect(run(`one\ntwo\nthr|ee`, ["d", "d"]).value).toBe("one\ntwo\n");
  });

  it("deletes to the end of the line with D", () => {
    expect(run("keep| this bit", ["D"]).value).toBe("keep");
  });

  it("clears a line and starts typing with cc", () => {
    const out = run(`one\nt|wo\nthree`, ["c", "c"]);
    expect(out.value).toBe("one\n\nthree");
    expect(out.mode).toBe("insert");
  });
});

describe("modes", () => {
  it("leaves typing on escape and does not file", () => {
    const out = run("some|thing", ["Escape"], "insert");
    expect(out.mode).toBe("normal");
    expect(out.exit).toBeUndefined();
  });

  it("files on escape from normal, which is what closing means everywhere else", () => {
    expect(run("some|thing", ["Escape"]).exit).toBe("file");
    expect(run("some|thing", ["Z", "Z"]).exit).toBe("file");
    expect(run("some|thing", ["Z", "Q"]).exit).toBe("scrap");
  });

  it("gets back to typing in the usual places", () => {
    expect(run("  ab|c", ["i"]).mode).toBe("insert");
    expect(run("ab|c", ["a"]).shown).toBe("abc|");
    expect(run("  ab|c", ["I"]).shown).toBe("  |abc");
    expect(run("a|bc", ["A"]).shown).toBe("abc|");
    expect(run("on|e\ntwo", ["o"]).value).toBe("one\n\ntwo");
    expect(run("one\nt|wo", ["O"]).value).toBe("one\n\ntwo");
  });

  it("swallows stray letters rather than typing them into the draft", () => {
    const out = press("q", { value: "hello", at: 5, mode: "normal", pending: "" });
    expect(out.handled).toBe(true);
    expect(out.value).toBe("hello");
  });

  it("leaves every key alone while typing", () => {
    for (const key of ["d", "w", "x", "G", "?"]) {
      expect(press(key, { value: "", at: 0, mode: "insert", pending: "" }).handled).toBe(false);
    }
  });
});

// The reference is served from internal/vim and both front ends are meant to
// answer all of it. A sheet that promises a key nobody wrote is worse than no
// sheet, so every press on it is checked here.
describe("the reference sheet", () => {
  const documented = [
    "esc", "i", "a", "I", "A", "o", "O",
    "h", "j", "k", "l", "0", "$", "^", "gg", "G",
    "w", "b", "e",
    "x", "dd", "D", "cc", "u",
    "ZZ", "ZQ", "?",
  ];

  it("answers every key it advertises", () => {
    for (const entry of documented) {
      const keys = entry === "esc" ? ["Escape"] : entry.split("");
      const out = run("one t|wo\nthree", keys);
      // u is deliberately the browser's own undo; everything else is ours.
      if (entry === "u") continue;
      const last = press(keys[keys.length - 1], {
        value: "one two\nthree", at: 5, mode: "normal",
        pending: keys.length > 1 ? keys[0] : "",
      });
      expect(last.handled, `${entry} is on the sheet but goes unanswered`).toBe(true);
      expect(out).toBeDefined();
    }
  });
});

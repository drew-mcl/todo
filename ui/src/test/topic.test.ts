import { describe, expect, it } from "vitest";
import { assignHues, topicColor, topicHue } from "../lib/topic";

describe("topic colours", () => {
  it("gives a topic the same colour every time", () => {
    expect(topicHue("prod issue")).toBe(topicHue("prod issue"));
    expect(topicColor("admin")).toBe(topicColor("admin"));
  });

  it("stays inside the curated set", () => {
    for (const t of ["a", "prod issue", "admin", "", "a much longer topic name", "🙂"]) {
      const hue = topicHue(t);
      expect(hue).toBeGreaterThanOrEqual(0);
      expect(hue).toBeLessThan(8);
    }
  });

  // The identical table is asserted in internal/palette/palette_test.go. If
  // either implementation drifts, one side fails and a topic stops being the
  // same colour in the terminal as it is here.
  it("agrees with the Go implementation", () => {
    const pinned: Record<string, number> = {
      "prod issue": 1,
      admin: 4,
      personal: 5,
      platform: 2,
      "some tool": 3,
      board: 5,
      inbox: 5,
      "": 5,
    };
    for (const [topic, hue] of Object.entries(pinned)) {
      expect(topicHue(topic)).toBe(hue);
    }
  });

  it("gives every topic on screen a colour of its own", () => {
    // board, inbox and personal all prefer hue 5.
    const topics = ["prod issue", "admin", "personal", "platform", "some tool", "board"];
    const hues = assignHues(topics);
    expect(new Set(hues.values()).size).toBe(topics.length);
  });

  it("does not depend on the order it was given", () => {
    const a = assignHues(["admin", "board", "personal"]);
    const b = assignHues(["personal", "admin", "board"]);
    for (const [k, v] of a) expect(b.get(k)).toBe(v);
  });

  it("points at a variable the stylesheet actually defines", () => {
    // Not the --color-* theme aliases: Tailwind drops those unless it sees a
    // matching utility class, which an inline style is not.
    expect(topicColor("admin")).toMatch(/^var\(--c-t[0-7]\)$/);
  });

  it("spreads topics across the palette", () => {
    const hues = new Set(
      ["prod issue", "admin", "personal", "platform", "some tool", "board"].map(topicHue),
    );
    expect(hues.size).toBeGreaterThan(2);
  });
});

import { describe, expect, it } from "vitest";
import { topicColor, topicHue } from "../lib/topic";

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

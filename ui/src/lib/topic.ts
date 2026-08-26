// Topics earn a colour, and keep it for as long as the set of topics does.
//
// The same rule lives in internal/palette/palette.go, so a topic is the same
// colour in the terminal as it is here. Tests on both sides pin the mapping.

export const HUES = 8;

/** A topic's preferred colour. FNV-1a, folded to 32 bits. */
export function topicHue(topic: string): number {
  let h = 2166136261;
  if (topic === "") {
    // h only becomes a signed 32-bit value once Math.imul has run, so with
    // nothing to hash it stays the plain seed.
    return h % HUES;
  }
  for (let i = 0; i < topic.length; i++) {
    h ^= topic.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return Math.abs(h) % HUES;
}

/**
 * Give every topic on screen a distinct colour.
 *
 * Hashing alone is not enough: with eight hues and half a dozen topics a
 * collision is more likely than not, and two topics sharing a dot defeats the
 * only reason the dots exist. The hash picks a preference; the next free hue is
 * taken when that preference is spoken for.
 *
 * Sorted first, so the answer does not depend on the order the caller held them
 * in.
 */
export function assignHues(topics: string[]): Map<string, number> {
  const out = new Map<string, number>();
  const taken = new Set<number>();
  for (const topic of [...topics].sort()) {
    let hue = topicHue(topic);
    for (let i = 0; i < HUES && taken.has(hue); i++) hue = (hue + 1) % HUES;
    taken.add(hue);
    out.set(topic, hue);
  }
  return out;
}

/**
 * The CSS variable for a hue.
 *
 * Points at the raw --c-t* variables rather than the --color-t* theme aliases:
 * Tailwind only emits a theme colour it can see used as a utility class, so the
 * aliases get tree-shaken out from under an inline style.
 */
export function hueVar(hue: number): string {
  return `var(--c-t${hue})`;
}

/** Fallback for when the full set of topics is not to hand. */
export function topicColor(topic: string): string {
  return hueVar(topicHue(topic));
}

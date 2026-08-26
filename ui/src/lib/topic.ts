// Topics earn a colour, and keep it for life.
//
// A stable hash picks from eight curated hues, so `prod issue` is the same dot
// every time you see it and the list becomes scannable without a legend. This
// is the one place colour is spent decoratively; everything else is signal.

const HUES = 8;

export function topicHue(topic: string): number {
  let h = 2166136261;
  for (let i = 0; i < topic.length; i++) {
    h ^= topic.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return Math.abs(h) % HUES;
}

/**
 * This topic's colour.
 *
 * Points at the raw --c-t* variables rather than the --color-t* theme aliases:
 * Tailwind only emits a theme colour it can see used as a utility class, so the
 * aliases get tree-shaken out from under an inline style.
 */
export function topicColor(topic: string): string {
  return `var(--c-t${topicHue(topic)})`;
}

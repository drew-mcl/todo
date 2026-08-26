// Package palette holds the few colours this app spends, and the rule for
// assigning one to a topic.
//
// The same values live in ui/src/theme.css and the same hash lives in
// ui/src/lib/topic.ts, so a topic is the same colour in the terminal as it is
// in the browser. Cross-language tests on both sides pin the mapping.
package palette

import "slices"

// Hues is how many topic colours there are.
const Hues = 8

// TopicHue is a topic's preferred colour. FNV-1a, folded exactly the way
// JavaScript folds it so Go and TypeScript agree.
//
// Iterating runes matches JavaScript's charCodeAt for everything in the basic
// plane, which is every topic anyone types.
func TopicHue(topic string) int {
	var h uint32 = 2166136261
	if topic == "" {
		// JavaScript's h only becomes a signed 32-bit value once Math.imul has
		// run, so with nothing to hash it stays the plain seed.
		return int(uint64(h) % Hues)
	}
	for _, c := range topic {
		h ^= uint32(c)
		h *= 16777619
	}
	// Math.abs on a signed 32-bit value, widened so the most negative one does
	// not overflow on the way back.
	v := int64(int32(h))
	if v < 0 {
		v = -v
	}
	return int(v % Hues)
}

// Assign gives every topic on screen a distinct colour.
//
// Hashing alone is not enough: with eight hues and half a dozen topics a
// collision is more likely than not, and two topics sharing a dot defeats the
// only reason the dots exist. So the hash picks a preference and the next free
// hue is taken when that preference is spoken for.
//
// Sorted first, so the answer does not depend on the order the caller happened
// to hold them in, and a topic keeps its colour as long as the set does.
func Assign(topics []string) map[string]int {
	sorted := slices.Clone(topics)
	slices.Sort(sorted)

	out := make(map[string]int, len(sorted))
	taken := make(map[int]bool, len(sorted))
	for _, t := range sorted {
		hue := TopicHue(t)
		for range Hues {
			if !taken[hue] {
				break
			}
			hue = (hue + 1) % Hues
		}
		// More topics than hues: past that point they must start sharing.
		taken[hue] = true
		out[t] = hue
	}
	return out
}

// Topic colours, muted enough that a dense list does not turn into a clown car.
var (
	TopicLight = [Hues]string{
		"#5B7FB9", "#3F8F86", "#6E8B4E", "#B07A33",
		"#B2634A", "#A85472", "#7A63B8", "#64748B",
	}
	TopicDark = [Hues]string{
		"#8FADE0", "#6FC4B8", "#A3C176", "#DCA85C",
		"#DE9179", "#DB90A8", "#A996E0", "#94A3B8",
	}
)

// Topic returns the light and dark hex for a topic, ready for an adaptive colour.
func Topic(topic string) (light, dark string) {
	i := TopicHue(topic)
	return TopicLight[i], TopicDark[i]
}

// The rest of the system. Light first, then dark.
const (
	InkLight, InkDark   = "#18181B", "#F4F4F5"
	Ink2Light, Ink2Dark = "#35353C", "#CBCBD2"
	Ink3Light, Ink3Dark = "#5B5B64", "#9A9AA4"
	Ink4Light, Ink4Dark = "#83838D", "#70707A"

	LineLight, LineDark = "#E4E4E7", "#27272A"
	SunkLight, SunkDark = "#F4F4F5", "#161618"

	AccentLight, AccentDark = "#0E7490", "#3BB0C4"
	DangerLight, DangerDark = "#B4423A", "#D97A70"
)

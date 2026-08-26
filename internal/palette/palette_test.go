package palette

import "testing"

// TestTopicHueIsStable pins the mapping. The identical table is asserted in
// ui/src/test/topic.test.ts -- if either implementation drifts, one of the two
// fails and a topic stops being the same colour across surfaces.
func TestTopicHueIsStable(t *testing.T) {
	// Taken from the shipping TypeScript, which is the older of the two.
	for topic, want := range map[string]int{
		"prod issue": 1,
		"admin":      4,
		"personal":   5,
		"platform":   2,
		"some tool":  3,
		"board":      5,
		"inbox":      5,
		"":           5,
	} {
		if got := TopicHue(topic); got != want {
			t.Errorf("TopicHue(%q) = %d, want %d — the browser and the terminal have drifted apart",
				topic, got, want)
		}
	}
}

func TestTopicHueStaysInRange(t *testing.T) {
	for _, s := range []string{"a", "", "a much longer topic than anyone would type", "ünïcode"} {
		if got := TopicHue(s); got < 0 || got >= Hues {
			t.Errorf("TopicHue(%q) = %d, outside 0..%d", s, got, Hues-1)
		}
	}
}

// TestAssignSeparatesTopics is the point of Assign: board, inbox and personal
// all prefer hue 5, and a list where three topics share a dot is a list where
// the dots say nothing.
func TestAssignSeparatesTopics(t *testing.T) {
	topics := []string{"prod issue", "admin", "personal", "platform", "some tool", "board"}
	got := Assign(topics)

	seen := map[int]string{}
	for _, topic := range topics {
		hue, ok := got[topic]
		if !ok {
			t.Fatalf("%q was not assigned a colour", topic)
		}
		if other, clash := seen[hue]; clash {
			t.Errorf("%q and %q were both given hue %d", topic, other, hue)
		}
		seen[hue] = topic
	}
}

func TestAssignIsOrderIndependent(t *testing.T) {
	a := Assign([]string{"admin", "board", "personal"})
	b := Assign([]string{"personal", "admin", "board"})
	for k, v := range a {
		if b[k] != v {
			t.Errorf("%q got hue %d one way and %d the other", k, v, b[k])
		}
	}
}

func TestAssignHandlesMoreTopicsThanHues(t *testing.T) {
	var many []string
	for i := range Hues * 2 {
		many = append(many, string(rune('a'+i))+"-topic")
	}
	got := Assign(many)
	if len(got) != len(many) {
		t.Errorf("assigned %d of %d topics", len(got), len(many))
	}
	for topic, hue := range got {
		if hue < 0 || hue >= Hues {
			t.Errorf("%q got hue %d, outside the palette", topic, hue)
		}
	}
}

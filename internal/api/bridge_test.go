package api

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/drew-mcl/todo/internal/palette"
	"github.com/drew-mcl/todo/internal/store"
)

// talk runs a conversation through the bridge and hands back the replies.
func talk(t *testing.T, reqs ...BridgeRequest) []BridgeReply {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	var in bytes.Buffer
	enc := json.NewEncoder(&in)
	for _, r := range reqs {
		if err := enc.Encode(r); err != nil {
			t.Fatalf("encoding a request: %v", err)
		}
	}

	var out bytes.Buffer
	if err := Bridge(st, func() time.Time { return now }, &in, &out); err != nil {
		t.Fatalf("Bridge: %v", err)
	}

	var replies []BridgeReply
	dec := json.NewDecoder(&out)
	for dec.More() {
		var r BridgeReply
		if err := dec.Decode(&r); err != nil {
			t.Fatalf("decoding a reply: %v", err)
		}
		replies = append(replies, r)
	}
	if len(replies) != len(reqs) {
		t.Fatalf("sent %d requests and got %d replies", len(reqs), len(replies))
	}
	return replies
}

// The bar draws nothing it was not told, so hello has to carry every colour it
// is allowed to use.
func TestBridgeHelloCarriesThePalette(t *testing.T) {
	got := talk(t, BridgeRequest{ID: 1, Op: "hello"})[0]
	if got.Error != "" {
		t.Fatalf("hello failed: %s", got.Error)
	}
	if got.Hello == nil {
		t.Fatal("no hello came back")
	}
	if n := len(got.Hello.Palette.Topic); n != palette.Hues {
		t.Errorf("sent %d topic colours, and there are %d", n, palette.Hues)
	}
	for _, key := range []string{"ink", "line", "accent", "danger", "sunk"} {
		c := got.Hello.Palette.Scheme[key]
		if c == nil || c.Light == "" || c.Dark == "" {
			t.Errorf("%s is missing an appearance: %+v", key, c)
		}
	}
	if got.Hello.Palette.Scheme["accent"].Light != palette.AccentLight {
		t.Error("the wire palette has drifted from internal/palette")
	}
}

// The same parse, the same highlighting and the same colour rule the other two
// front ends get: the bar is a third client, not a second implementation.
func TestBridgePreviewsLikeTheBrowser(t *testing.T) {
	draft := "prod issue | chase the vendor | today @sam !!\n| write the postmortem | eow\nnot a task line"
	got := talk(t, BridgeRequest{ID: 1, Op: "preview", Draft: draft})[0]
	if got.Preview == nil {
		t.Fatalf("no preview came back: %s", got.Error)
	}
	if got.Preview.Tasks != 2 || got.Preview.Skipped != 1 {
		t.Errorf("read %d tasks and %d skipped", got.Preview.Tasks, got.Preview.Skipped)
	}

	first := got.Preview.Lines[0]
	if first.Task == nil || first.Task.Title != "chase the vendor" {
		t.Fatalf("first line read as %+v", first.Task)
	}
	if first.Task.DueLabel == "" {
		t.Error("the date was not resolved for the bar, so the bar would have to")
	}
	var pipes int
	for _, tok := range first.Tokens {
		if tok.Kind == "pipe" {
			pipes++
		}
	}
	if pipes != 2 {
		t.Errorf("the line came back with %d separators coloured", pipes)
	}

	// The ditto line takes the topic above it, and both share one dot.
	if got.Preview.Lines[1].Task.Topic != "prod issue" {
		t.Errorf("the ditto line was filed under %q", got.Preview.Lines[1].Task.Topic)
	}
	if _, ok := got.Hues["prod issue"]; !ok {
		t.Errorf("no colour was assigned for the topic on screen: %v", got.Hues)
	}
}

// Two topics on one screen must not share a dot, which is Assign's job and not
// the bar's.
func TestBridgeHuesAreDistinct(t *testing.T) {
	got := talk(t, BridgeRequest{ID: 1, Op: "preview",
		Draft: "one | a\ntwo | b\nthree | c\nfour | d"})[0]

	seen := map[int]string{}
	for topic, hue := range got.Hues {
		if other, clash := seen[hue]; clash {
			t.Errorf("%q and %q were given the same dot", topic, other)
		}
		seen[hue] = topic
	}
}

func TestBridgeCapturesAndUndoes(t *testing.T) {
	replies := talk(t,
		BridgeRequest{ID: 1, Op: "capture", Draft: "admin | pull the numbers | today", Title: "Platform sync"},
		BridgeRequest{ID: 2, Op: "capture", Draft: "nothing here contains a separator"},
	)

	added := replies[0].Added
	if added == nil {
		t.Fatalf("nothing was captured: %s", replies[0].Error)
	}
	if added.Added != 1 || added.BatchID == 0 {
		t.Errorf("captured %+v", added)
	}
	if added.Today != 1 {
		t.Errorf("the day came back as %d, so the bar cannot say what it did", added.Today)
	}

	if replies[1].Error == "" {
		t.Error("a draft with no separator was accepted")
	}
	if !strings.Contains(replies[1].Error, "|") {
		t.Errorf("the refusal does not mention the separator: %q", replies[1].Error)
	}

	// Undo reaches back into the same store the other two front ends use.
	undo := talk(t,
		BridgeRequest{ID: 1, Op: "capture", Draft: "admin | pull the numbers | today"},
		BridgeRequest{ID: 2, Op: "undo", Batch: 1},
	)[1]
	if undo.Undone == nil || *undo.Undone != 1 {
		t.Errorf("undo returned %v (%s)", undo.Undone, undo.Error)
	}
}

// An unknown op is answered rather than dropped, so a bar built against a newer
// binary does not simply hang.
func TestBridgeAnswersEveryRequest(t *testing.T) {
	got := talk(t, BridgeRequest{ID: 7, Op: "fly"})[0]
	if got.ID != 7 {
		t.Errorf("the reply is tagged %d, not 7", got.ID)
	}
	if got.Error == "" {
		t.Error("an unknown op was answered as though it worked")
	}
}

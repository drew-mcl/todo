package api

// What a request costs against a list far larger than anyone's.
//
//	go test ./internal/api -run xxx -bench . -benchtime 30x

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/drew-mcl/todo/internal/parse"
	"github.com/drew-mcl/todo/internal/store"
)

func loaded(b *testing.B, tasks int) (*Server, *store.Store) {
	b.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { st.Close() })
	var draft strings.Builder
	for i := range tasks {
		fmt.Fprintf(&draft, "topic %d | something that needs doing | today @person%d !! #tag%d\n",
			i%12, i%9, i%7)
	}
	if _, err := st.CreateBatch(parse.Parse(draft.String(), now).Tasks,
		store.Capture{Source: "bench"}, now); err != nil {
		b.Fatal(err)
	}
	return New(st, func() time.Time { return now }, nil), st
}

func BenchmarkCounts(b *testing.B) {
	_, st := loaded(b, 2000)
	b.ResetTimer()
	for b.Loop() {
		if _, err := st.Counts(now); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMeta(b *testing.B) {
	srv, _ := loaded(b, 2000)
	b.ResetTimer()
	for b.Loop() {
		if _, err := srv.meta(now); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkListEndpoint(b *testing.B) {
	srv, _ := loaded(b, 2000)
	b.ResetTimer()
	for b.Loop() {
		r := httptest.NewRequest("GET", "/api/list?view=all", nil)
		r.Host = "127.0.0.1:8765"
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		if w.Code != 200 {
			b.Fatalf("status %d", w.Code)
		}
	}
}

func BenchmarkPreviewEndpoint(b *testing.B) {
	srv, _ := loaded(b, 100)
	var draft strings.Builder
	for i := range 200 {
		fmt.Fprintf(&draft, "topic %d | something that needs doing | today @sam !!\n", i%12)
	}
	body := draft.String()
	b.ResetTimer()
	for b.Loop() {
		r := httptest.NewRequest("POST", "/api/preview",
			strings.NewReader(`{"draft":`+quote(body)+`}`))
		r.Host = "127.0.0.1:8765"
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		if w.Code != 200 {
			b.Fatalf("status %d", w.Code)
		}
	}
}

func quote(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `"`, `\"`), "\n", `\n`) + `"`
}

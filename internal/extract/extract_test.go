package extract

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// TestGolden extracts every page in testdata and compares the draft with its
// .json golden file. Run with -update after a deliberate change.
func TestGolden(t *testing.T) {
	pages, _ := filepath.Glob("testdata/*.html")
	if len(pages) == 0 {
		t.Fatal("no fixtures")
	}
	for _, page := range pages {
		t.Run(filepath.Base(page), func(t *testing.T) {
			body, err := os.ReadFile(page)
			if err != nil {
				t.Fatal(err)
			}
			contentType := "text/html; charset=utf-8"
			if strings.Contains(page, "latin1") {
				contentType = "text/html; charset=iso-8859-1"
			}
			d, err := Extract(body, contentType, "https://theatre.example/spectacle/")
			if err != nil {
				t.Fatal(err)
			}
			got, _ := json.MarshalIndent(struct {
				*Draft
				Text string `json:"text"`
			}{d, d.Text}, "", "  ")
			golden := strings.TrimSuffix(page, ".html") + ".json"
			if *update {
				os.WriteFile(golden, append(got, '\n'), 0o644)
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("missing golden file (run go test -update): %v", err)
			}
			if strings.TrimSpace(string(want)) != strings.TrimSpace(string(got)) {
				t.Errorf("draft changed:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestParseURL(t *testing.T) {
	for _, bad := range []string{"ftp://x.example/", "javascript:alert(1)", "https://user:pw@x.example/", "http://"} {
		if _, err := ParseURL(bad); err == nil {
			t.Errorf("ParseURL(%q) accepted", bad)
		}
	}
	u, err := ParseURL("  www.theatre.example/spectacle#top ")
	if err != nil || u.String() != "https://www.theatre.example/spectacle" {
		t.Errorf("ParseURL = %v, %v", u, err)
	}
}

func TestFetcherRefusesLocalAddresses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("secret")) }))
	defer srv.Close()
	_, err := NewFetcher(false).Get(context.Background(), srv.URL)
	if !errors.Is(err, ErrForbiddenAddress) {
		t.Fatalf("err = %v, want ErrForbiddenAddress", err)
	}
	if _, err := NewFetcher(true).Get(context.Background(), srv.URL); err != nil {
		t.Fatalf("allowed fetch failed: %v", err)
	}
}

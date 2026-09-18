package static

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestHandlerFor(t *testing.T) {
	const indexHTML = "<!doctype html><title>CMS</title><div id=root></div>"
	const assetJS = "console.log('app')"

	dist := fstest.MapFS{
		"index.html":        {Data: []byte(indexHTML)},
		"assets/app-abc.js": {Data: []byte(assetJS)},
	}

	h, ok := handlerFor(dist)
	if !ok {
		t.Fatal("expected handler to activate when index.html is present")
	}

	do := func(path string) *http.Response {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Result()
	}

	// Root serves index.html.
	if res := do("/"); res.StatusCode != 200 {
		t.Errorf("GET / = %d, want 200", res.StatusCode)
	} else if b, _ := io.ReadAll(res.Body); string(b) != indexHTML {
		t.Errorf("GET / body = %q, want index.html", string(b))
	}

	// A client-side route falls back to index.html.
	res := do("/content/pages")
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 || string(b) != indexHTML {
		t.Errorf("GET /content/pages = %d %q, want 200 index.html", res.StatusCode, string(b))
	}

	// A real asset is served with an immutable cache header.
	res = do("/assets/app-abc.js")
	b, _ = io.ReadAll(res.Body)
	if res.StatusCode != 200 || string(b) != assetJS {
		t.Errorf("GET asset = %d %q, want 200 asset", res.StatusCode, string(b))
	}
	if cc := res.Header.Get("Cache-Control"); cc == "" || cc == "no-cache" {
		t.Errorf("asset Cache-Control = %q, want immutable long cache", cc)
	}
}

func TestHandlerFor_NoBuild(t *testing.T) {
	// No index.html → API-only: handler must not activate.
	if _, ok := handlerFor(fstest.MapFS{"placeholder": {Data: []byte("x")}}); ok {
		t.Fatal("expected handler to stay inactive without index.html")
	}
}

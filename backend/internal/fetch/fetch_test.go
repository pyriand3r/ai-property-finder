package fetch

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

const testListingHTML = `<!DOCTYPE html>
<html>
<head><title>Kappmuren 2 — Malmö</title></head>
<body>
<nav><a href="/other-listing">Andere Anzeige</a></nav>
<article>
<h1>Kappmuren 2</h1>
<p>Ein wunderschönes Grundstück mit Blick auf den See, gelegen in ruhiger Umgebung
nahe Malmö. Das Haus verfügt über drei Schlafzimmer, ein modernes Bad und einen
großzügigen Garten mit altem Baumbestand. Die Küche wurde 2019 renoviert und
bietet viel Stauraum sowie hochwertige Geräte. Ideal für Familien, die Ruhe und
Natur schätzen, aber dennoch die Nähe zur Stadt genießen möchten.</p>
<img src="/photos/front.jpg" alt="Front">
<img src="/photos/kitchen.jpg" alt="Kitchen">
<a href="https://maklare.example.com/kappmuren-2">Läs mer hos mäklaren</a>
</article>
</body>
</html>`

func TestFetchPage_ExtractsTextImagesAndLinks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(testListingHTML))
	}))
	defer server.Close()

	client := &Client{HTTPClient: server.Client()}
	result, err := client.FetchPage(context.Background(), server.URL+"/listing")
	if err != nil {
		t.Fatalf("FetchPage returned error: %v", err)
	}

	if result.Text == "" {
		t.Error("Text is empty, want extracted article text")
	}
	if !strings.Contains(result.Text, "Grundstück") {
		t.Errorf("Text = %q, want it to contain the body paragraph", result.Text)
	}
	if !strings.Contains(result.Title, "Kappmuren") {
		t.Errorf("Title = %q, want it to mention Kappmuren", result.Title)
	}

	wantImages := []string{server.URL + "/photos/front.jpg", server.URL + "/photos/kitchen.jpg"}
	if !slices.Equal(result.ImageURLs, wantImages) {
		t.Errorf("ImageURLs = %v, want %v", result.ImageURLs, wantImages)
	}

	wantLinks := []string{server.URL + "/other-listing", "https://maklare.example.com/kappmuren-2"}
	if !slices.Equal(result.Links, wantLinks) {
		t.Errorf("Links = %v, want %v", result.Links, wantLinks)
	}
}

func TestFetchPage_RejectsNonHTTPScheme(t *testing.T) {
	client := NewClient()
	if _, err := client.FetchPage(context.Background(), "ftp://example.com/file"); err == nil {
		t.Error("expected an error for a non-http(s) URL")
	}
}

func TestFetchPage_PropagatesNon200Status(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := &Client{HTTPClient: server.Client()}
	if _, err := client.FetchPage(context.Background(), server.URL); err == nil {
		t.Error("expected an error for a 403 response")
	}
}

func TestDownloadImage_ExtensionFromURLPath(t *testing.T) {
	imageBytes := []byte{0xFF, 0xD8, 0xFF} // not a full valid JPEG, extension comes from the URL path here
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(imageBytes)
	}))
	defer server.Close()

	client := &Client{HTTPClient: server.Client()}
	image, err := client.DownloadImage(context.Background(), server.URL+"/photos/front.jpg")
	if err != nil {
		t.Fatalf("DownloadImage returned error: %v", err)
	}
	if image.Extension != ".jpg" {
		t.Errorf("Extension = %q, want .jpg", image.Extension)
	}
	if !bytes.Equal(image.Data, imageBytes) {
		t.Errorf("Data = %v, want %v", image.Data, imageBytes)
	}
}

func TestDownloadImage_ExtensionFromContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/webp")
		_, _ = w.Write([]byte("fake-webp-bytes"))
	}))
	defer server.Close()

	client := &Client{HTTPClient: server.Client()}
	image, err := client.DownloadImage(context.Background(), server.URL+"/image-endpoint")
	if err != nil {
		t.Fatalf("DownloadImage returned error: %v", err)
	}
	if image.Extension != ".webp" {
		t.Errorf("Extension = %q, want .webp", image.Extension)
	}
}

func TestDownloadImage_RejectsUnrecognizedFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.4"))
	}))
	defer server.Close()

	client := &Client{HTTPClient: server.Client()}
	if _, err := client.DownloadImage(context.Background(), server.URL+"/document"); err == nil {
		t.Error("expected an error for a non-image content type")
	}
}

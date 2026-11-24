// Package fetch is the plain-HTTP page/PDF/image loader backing the Agent's
// fetch_url and download_image Tools (spec 00-03). No headless browser —
// chromedp is explicitly deferred past MVP (AGENTS.md "Deferred / Not in
// MVP"), so anti-bot-walled pages surface as a fetch error and the Agent
// falls back to asking the user for a PDF (spec 01-03 "Failure Modes").
package fetch

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	nurl "net/url"
	"path"
	"strings"
	"time"

	readability "codeberg.org/readeck/go-readability/v2"
	"golang.org/x/net/html"
)

const (
	fetchTimeout = 20 * time.Second
	// maxBodyBytes caps a single response — a listing page or photo never
	// legitimately exceeds this; it bounds a pathological/malicious response
	// instead of buffering it fully into memory.
	maxBodyBytes = 8 << 20
	// userAgent presents as a normal desktop browser. Real estate portals
	// commonly block Go's default UA outright; this is fetching pages the
	// user already has access to view, for their own single-user tool, not
	// evading any access control.
	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
)

// Result is fetch_url's output: the page's readable text plus every image
// and outbound link discovered in the raw HTML. Images/links are collected
// from the full document, not just the extracted article — a portal's
// "read more at the broker's site" link (the secondary link a Portal Rule
// tells the Agent to follow) or a photo gallery's thumbnails often sit
// outside what Readability keeps as "article content". The Agent, not this
// package, judges which are relevant.
type Result struct {
	Title     string
	Text      string
	ImageURLs []string
	Links     []string
	// PDFData contains the raw response when the URL is a PDF. The Agent
	// sends it as a native document instead of running HTML extraction.
	PDFData []byte
}

// DownloadedImage is download_image's fetched payload, ready to write to
// disk under a fresh UUID name (ADR 0012). Extension includes the leading
// dot.
type DownloadedImage struct {
	Data      []byte
	Extension string
}

// Client fetches pages and images over plain HTTP. HTTPClient is exported
// so tests can point it at an httptest server's client instead of
// stubbing DNS/TLS.
type Client struct {
	HTTPClient *http.Client
}

// NewClient constructs a Client with the default fetch timeout.
func NewClient() *Client {
	return &Client{HTTPClient: &http.Client{Timeout: fetchTimeout}}
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: fetchTimeout}
}

// FetchPage backs the fetch_url Tool.
func (c *Client) FetchPage(ctx context.Context, pageURL string) (*Result, error) {
	parsedURL, body, contentType, err := c.fetchBytes(ctx, pageURL)
	if err != nil {
		return nil, err
	}
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if strings.EqualFold(mediaType, "application/pdf") || bytes.HasPrefix(bytes.TrimSpace(body), []byte("%PDF-")) {
		return &Result{PDFData: body}, nil
	}

	article, err := readability.FromReader(bytes.NewReader(body), parsedURL)
	if err != nil {
		return nil, fmt.Errorf("extract readable content from %s: %w", pageURL, err)
	}
	var text strings.Builder
	if article.Node != nil {
		if renderErr := article.RenderText(&text); renderErr != nil {
			return nil, fmt.Errorf("render article text from %s: %w", pageURL, renderErr)
		}
	}

	root, parseErr := html.Parse(bytes.NewReader(body))
	if parseErr != nil {
		return nil, fmt.Errorf("parse html from %s: %w", pageURL, parseErr)
	}

	return &Result{
		Title:     article.Title(),
		Text:      strings.TrimSpace(text.String()),
		ImageURLs: extractURLs(root, "img", "src", parsedURL),
		Links:     extractURLs(root, "a", "href", parsedURL),
	}, nil
}

// DownloadImage backs the download_image Tool.
func (c *Client) DownloadImage(ctx context.Context, imageURL string) (*DownloadedImage, error) {
	_, body, contentType, err := c.fetchBytes(ctx, imageURL)
	if err != nil {
		return nil, err
	}
	ext := extensionFor(imageURL, contentType)
	if ext == "" {
		return nil, fmt.Errorf("unrecognized image format for %s (content-type %q)", imageURL, contentType)
	}
	return &DownloadedImage{Data: body, Extension: ext}, nil
}

// fetchBytes GETs rawURL and returns its parsed URL, body (capped at
// maxBodyBytes), and Content-Type header — the shared plumbing behind both
// Tools, which differ only in what they do with the bytes.
func (c *Client) fetchBytes(ctx context.Context, rawURL string) (parsedURL *nurl.URL, body []byte, contentType string, err error) {
	parsedURL, err = nurl.ParseRequestURI(rawURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return nil, nil, "", fmt.Errorf("invalid url: %q", rawURL)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return nil, nil, "", fmt.Errorf("build request for %s: %w", rawURL, err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, nil, "", fmt.Errorf("fetch %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, "", fmt.Errorf("fetch %s: unexpected status %s", rawURL, resp.Status)
	}

	body, err = io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return nil, nil, "", fmt.Errorf("read response body from %s: %w", rawURL, err)
	}
	if len(body) > maxBodyBytes {
		return nil, nil, "", fmt.Errorf("response body from %s exceeds %d bytes", rawURL, maxBodyBytes)
	}
	return parsedURL, body, resp.Header.Get("Content-Type"), nil
}

var imageExtensionsByContentType = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

var validImageExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true,
}

// extensionFor returns a whitelisted, lowercased extension (leading dot),
// preferring the URL path's own extension and falling back to sniffing
// the response's Content-Type. Mirrors internal/service's
// safeImageExtension (the upload path) — duplicated rather than shared
// since the two live in different layers with different inputs (a
// filename+sniffed-bytes vs. a URL+header) and the overlap is a handful of
// lines.
func extensionFor(rawURL, contentType string) string {
	if parsed, err := nurl.Parse(rawURL); err == nil {
		ext := strings.ToLower(path.Ext(parsed.Path))
		if validImageExtensions[ext] {
			if ext == ".jpeg" {
				return ".jpg"
			}
			return ext
		}
	}
	if mapped, ok := imageExtensionsByContentType[strings.SplitN(contentType, ";", 2)[0]]; ok {
		return mapped
	}
	return ""
}

// extractURLs walks the full document collecting every non-empty attr
// value on tag, resolved to an absolute URL against base and deduplicated
// in first-seen order.
func extractURLs(node *html.Node, tag, attr string, base *nurl.URL) []string {
	var found []string
	seen := make(map[string]bool)

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == tag {
			for _, a := range n.Attr {
				if a.Key != attr {
					continue
				}
				resolved := resolveURL(base, a.Val)
				if resolved != "" && !seen[resolved] {
					seen[resolved] = true
					found = append(found, resolved)
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return found
}

func resolveURL(base *nurl.URL, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	switch {
	case strings.HasPrefix(ref, "javascript:"),
		strings.HasPrefix(ref, "data:"),
		strings.HasPrefix(ref, "mailto:"),
		strings.HasPrefix(ref, "tel:"),
		strings.HasPrefix(ref, "#"):
		return ""
	}
	refURL, err := nurl.Parse(ref)
	if err != nil {
		return ""
	}
	return base.ResolveReference(refURL).String()
}

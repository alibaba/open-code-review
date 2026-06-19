package main

import "testing"

func TestParseViewerFlagsReviewsDir(t *testing.T) {
	opts, err := parseViewerFlags([]string{"--addr", ":3000", "--reviews-dir", "/mnt/ocr/reviews"})
	if err != nil {
		t.Fatalf("parseViewerFlags: %v", err)
	}
	if opts.addr != ":3000" {
		t.Errorf("addr = %q", opts.addr)
	}
	if opts.reviewsDir != "/mnt/ocr/reviews" {
		t.Errorf("reviewsDir = %q", opts.reviewsDir)
	}
}

func TestParseViewerFlagsReviewsDirFromEnv(t *testing.T) {
	t.Setenv("OCR_REVIEWS_DIR", "/mnt/ocr/reviews")
	opts, err := parseViewerFlags(nil)
	if err != nil {
		t.Fatalf("parseViewerFlags: %v", err)
	}
	if opts.reviewsDir != "/mnt/ocr/reviews" {
		t.Errorf("reviewsDir = %q", opts.reviewsDir)
	}
}

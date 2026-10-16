package proofscreen

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestUploadSendsJSON(t *testing.T) {
	client := NewInfraiClient("test-key")
	client.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", got)
		}
		var body map[string]json.RawMessage
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatalf("decode upload body: %v", err)
		}
		if len(body) != 3 || body["file"] == nil || body["filename"] == nil || body["idempotency_key"] == nil {
			t.Fatalf("upload fields = %v", body)
		}
		var file []byte
		if err := json.Unmarshal(body["file"], &file); err != nil {
			t.Fatalf("decode file: %v", err)
		}
		if string(file) != "image bytes" {
			t.Fatalf("file = %q", file)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"ok":true,"data":{"url":"https://media.example/raw"}}`)),
		}, nil
	})

	got, err := client.Upload(context.Background(), ShipmentEvent{Filename: "proof.jpg", Image: []byte("image bytes")}, "req-17")
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if got != "https://media.example/raw" {
		t.Fatalf("Upload() = %q", got)
	}
}

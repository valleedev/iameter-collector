package httpclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPostJSONSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("X-Custom") != "value" {
			t.Errorf("X-Custom = %q, want value", r.Header.Get("X-Custom"))
		}
		if r.Header.Get("User-Agent") == "" {
			t.Error("User-Agent header missing")
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["hello"] != "world" {
			t.Errorf("body = %v", body)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	resp, err := c.PostJSON(context.Background(), "/v1/test", map[string]string{"hello": "world"}, map[string]string{"X-Custom": "value"})
	if err != nil {
		t.Fatalf("PostJSON() error = %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("StatusCode = %d, want 201", resp.StatusCode)
	}
	if string(resp.Body) != `{"ok":true}` {
		t.Errorf("Body = %q", resp.Body)
	}
}

func TestPostJSONRetryAfterSeconds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := New(srv.URL)
	resp, err := c.PostJSON(context.Background(), "/v1/test", map[string]string{}, nil)
	if err != nil {
		t.Fatalf("PostJSON() error = %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("StatusCode = %d, want 429", resp.StatusCode)
	}
	if resp.RetryAfter < 29*time.Second || resp.RetryAfter > 31*time.Second {
		t.Errorf("RetryAfter = %v, want ~30s", resp.RetryAfter)
	}
}

func TestPostJSONRetryAfterHTTPDate(t *testing.T) {
	future := time.Now().Add(1 * time.Minute).UTC()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", future.Format(http.TimeFormat))
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := New(srv.URL)
	resp, err := c.PostJSON(context.Background(), "/v1/test", map[string]string{}, nil)
	if err != nil {
		t.Fatalf("PostJSON() error = %v", err)
	}
	if resp.RetryAfter <= 0 || resp.RetryAfter > 61*time.Second {
		t.Errorf("RetryAfter = %v, want ~60s", resp.RetryAfter)
	}
}

func TestPostJSONNetworkError(t *testing.T) {
	c := New("http://127.0.0.1:1") // nothing listens here
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := c.PostJSON(ctx, "/v1/test", map[string]string{}, nil)
	if err == nil {
		t.Fatal("PostJSON() error = nil, want error for unreachable server")
	}
}

func TestPostJSONTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()

	c := New(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := c.PostJSON(ctx, "/v1/test", map[string]string{}, nil)
	if err == nil {
		t.Fatal("PostJSON() error = nil, want timeout error")
	}
}

func TestPostJSONOversizedResponseRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(make([]byte, maxTestResponseSize))
	}))
	defer srv.Close()

	c := New(srv.URL)
	_, err := c.PostJSON(context.Background(), "/v1/test", map[string]string{}, nil)
	if err == nil {
		t.Fatal("PostJSON() error = nil, want error for oversized response")
	}
}

const maxTestResponseSize = (1 << 20) + 1

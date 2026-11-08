package hikvision

import (
	"bytes"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestSnapshotHTTPRefreshFailureAndRecovery(t *testing.T) {
	camera := newFakeCamera(t)
	mux := http.NewServeMux()
	Plugin{client: camera.client(t, testPassword)}.RegisterHTTP(mux)
	get := func(method string) *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(method, snapshotEndpoint, nil))
		return response
	}
	first := get("GET")
	if first.Code != 200 || first.Header().Get("Content-Type") != "image/jpeg" || first.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("first response: %d %v", first.Code, first.Header())
	}
	if _, err := time.Parse(time.RFC3339Nano, first.Header().Get("X-Snapshot-Captured-At")); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		cached := get("GET")
		if !bytes.Equal(cached.Body.Bytes(), first.Body.Bytes()) || cached.Header().Get("X-Snapshot-Captured-At") != first.Header().Get("X-Snapshot-Captured-At") {
			t.Fatal("cache changed picture or timestamp")
		}
	}
	if camera.authorisedRequests.Load() != 1 {
		t.Fatal("dashboard refreshes fetched multiple pictures")
	}
	if head := get("HEAD"); head.Code != 200 || head.Body.Len() != 0 {
		t.Fatal("HEAD must return headers only")
	}
	if post := get("POST"); post.Code != 405 {
		t.Fatal("POST accepted")
	}
	camera.unavailable.Store(true)
	time.Sleep(snapshotCacheDuration + 50*time.Millisecond)
	failed := get("GET")
	if failed.Code != 502 || failed.Header().Get("X-Snapshot-Captured-At") != "" || bytes.Equal(failed.Body.Bytes(), first.Body.Bytes()) {
		t.Fatal("failed refresh served a picture")
	}
	camera.unavailable.Store(false)
	recovered := get("GET")
	if recovered.Code != 200 || recovered.Header().Get("X-Snapshot-Captured-At") == first.Header().Get("X-Snapshot-Captured-At") {
		t.Fatal("picture did not recover")
	}
}

// Opt-in read-only probe. The output is temporary evidence for visual inspection.
func TestLiveSnapshotHTTP(t *testing.T) {
	baseURL := os.Getenv("HIKVISION_BASE_URL")
	if baseURL == "" {
		t.Skip("set HIKVISION_BASE_URL, HIKVISION_CREDENTIALS_FILE and HIKVISION_PICTURE_OUTPUT")
	}
	raw, err := os.ReadFile(os.Getenv("HIKVISION_CREDENTIALS_FILE"))
	if err != nil {
		t.Fatal(err)
	}
	username, password, err := parseCredentials(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(Config{BaseURL: baseURL, Username: username, Password: password})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Plugin{client: client}.RegisterHTTP(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest("GET", snapshotEndpoint, nil))
	if response.Code != 200 {
		t.Fatalf("snapshot handler: %d %s", response.Code, response.Body.String())
	}
	picture, err := jpeg.Decode(bytes.NewReader(response.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("HIKVISION_PICTURE_OUTPUT"), response.Body.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("HTTP 200 image/jpeg; %dx%d; captured at %s", picture.Bounds().Dx(), picture.Bounds().Dy(), response.Header().Get("X-Snapshot-Captured-At"))
}

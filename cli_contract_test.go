package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

type capturedRequest struct {
	Method string
	Path   string
	Query  string
	Auth   string
	Body   map[string]any
}

func runTestCLI(t *testing.T, args []string, env map[string]string) (int, string, string) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), args, RunOptions{
		Env:    env,
		Stdout: &stdout,
		Stderr: &stderr,
	})
	return code, stdout.String(), stderr.String()
}

func newAPIServer(t *testing.T, handler func(capturedRequest) (int, string)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if r.Body != nil {
			data, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			if len(bytes.TrimSpace(data)) > 0 {
				if err := json.Unmarshal(data, &body); err != nil {
					t.Fatalf("request body is not json: %s: %v", string(data), err)
				}
			}
		}
		status, response := handler(capturedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.RawQuery,
			Auth:   r.Header.Get("Authorization"),
			Body:   body,
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
}

func TestEndpointRegistryCoversEveryOpenAPIOperation(t *testing.T) {
	expected := []string{
		"DELETE /characters/{character_id}",
		"DELETE /objects/{object_id}",
		"GET /background-jobs/{job_id}",
		"GET /balance",
		"GET /characters",
		"GET /characters/{character_id}",
		"GET /characters/{character_id}/zip",
		"GET /isometric-tiles",
		"GET /isometric-tiles/{tile_id}",
		"GET /llms.txt",
		"GET /objects",
		"GET /objects/{object_id}",
		"GET /tiles-pro/{tile_id}",
		"GET /tilesets",
		"GET /tilesets/{tileset_id}",
		"PATCH /characters/{character_id}/tags",
		"PATCH /objects/{object_id}/tags",
		"POST /animate-character",
		"POST /animate-with-skeleton",
		"POST /animate-with-text",
		"POST /animate-with-text-v2",
		"POST /animate-with-text-v3",
		"POST /characters/animations",
		"POST /create-1-direction-object",
		"POST /create-8-direction-object",
		"POST /create-character-pro",
		"POST /create-character-state",
		"POST /create-character-v3",
		"POST /create-character-with-4-directions",
		"POST /create-character-with-8-directions",
		"POST /create-image-bitforge",
		"POST /create-image-pixen",
		"POST /create-image-pixflux",
		"POST /create-isometric-tile",
		"POST /create-tiles-pro",
		"POST /create-tileset",
		"POST /create-tileset-sidescroller",
		"POST /edit-animation-v2",
		"POST /edit-image",
		"POST /edit-images-v2",
		"POST /enhance-animation-v3-prompt",
		"POST /enhance-character-v3-prompt",
		"POST /enhance-pixen-prompt",
		"POST /estimate-skeleton",
		"POST /generate-8-rotations-v2",
		"POST /generate-8-rotations-v3",
		"POST /generate-image-v2",
		"POST /generate-ui-v2",
		"POST /generate-with-style-v2",
		"POST /image-to-pixelart",
		"POST /inpaint",
		"POST /inpaint-v3",
		"POST /interpolation-v2",
		"POST /map-objects",
		"POST /objects/{object_id}/animations",
		"POST /objects/{object_id}/dismiss-review",
		"POST /objects/{object_id}/select-frames",
		"POST /objects/{object_id}/states",
		"POST /remove-background",
		"POST /resize",
		"POST /rotate",
		"POST /tilesets",
		"POST /tilesets-sidescroller",
		"POST /transfer-outfit-v2",
	}

	var actual []string
	for _, spec := range EndpointSpecs() {
		actual = append(actual, spec.Method+" "+spec.Path)
	}
	sort.Strings(expected)
	sort.Strings(actual)
	if strings.Join(actual, "\n") != strings.Join(expected, "\n") {
		t.Fatalf("endpoint registry mismatch\nexpected:\n%s\nactual:\n%s", strings.Join(expected, "\n"), strings.Join(actual, "\n"))
	}
}

func TestGetBalanceSendsAuthAndNoBody(t *testing.T) {
	server := newAPIServer(t, func(req capturedRequest) (int, string) {
		if req.Method != http.MethodGet || req.Path != "/balance" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.Path)
		}
		if req.Auth != "Bearer test-token" {
			t.Fatalf("missing auth header: %q", req.Auth)
		}
		if req.Body != nil {
			t.Fatalf("GET /balance should not send a body: %#v", req.Body)
		}
		return http.StatusOK, `{"credits":12.5,"generations":42}`
	})
	defer server.Close()

	code, stdout, stderr := runTestCLI(t, []string{"get", "/balance", "--base-url", server.URL}, map[string]string{"PIXELLAB_API_KEY": "test-token"})
	if code != 0 {
		t.Fatalf("expected success, got code %d stderr %s", code, stderr)
	}
	if !strings.Contains(stdout, "credits") || !strings.Contains(stdout, "generations") {
		t.Fatalf("expected balance output, got %q", stdout)
	}
}

func TestPostPixenBuildsTypedJSONBody(t *testing.T) {
	server := newAPIServer(t, func(req capturedRequest) (int, string) {
		if req.Method != http.MethodPost || req.Path != "/create-image-pixen" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.Path)
		}
		assertJSONValue(t, req.Body, "description", "cute dragon")
		assertJSONValue(t, req.Body, "outline", "selective outline")
		assertJSONValue(t, req.Body, "no_background", true)
		assertJSONValue(t, req.Body, "enhance_prompt", true)
		assertJSONValue(t, req.Body, "seed", float64(123))
		assertSizeObject(t, req.Body["image_size"], 128, 64)
		return http.StatusOK, `{"images":[{"base64":"abcd","format":"png"}]}`
	})
	defer server.Close()

	code, _, stderr := runTestCLI(t, []string{
		"post", "/create-image-pixen",
		"--base-url", server.URL,
		"--description", "cute dragon",
		"--image-size", "128x64",
		"--outline", "selective outline",
		"--no-background",
		"--enhance-prompt",
		"--seed", "123",
	}, map[string]string{"PIXELLAB_API_KEY": "test-token"})
	if code != 0 {
		t.Fatalf("expected success, got code %d stderr %s", code, stderr)
	}
}

func TestPathParamsQueryParamsAndPatchTags(t *testing.T) {
	server := newAPIServer(t, func(req capturedRequest) (int, string) {
		if req.Method != http.MethodPatch || req.Path != "/objects/object-123/tags" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.Path)
		}
		tags, ok := req.Body["tags"].([]any)
		if !ok || len(tags) != 2 || tags[0] != "prop" || tags[1] != "barrel" {
			t.Fatalf("unexpected tags body: %#v", req.Body)
		}
		return http.StatusOK, `{"ok":true}`
	})
	defer server.Close()

	code, _, stderr := runTestCLI(t, []string{
		"patch", "/objects/{object_id}/tags",
		"--base-url", server.URL,
		"--object-id", "object-123",
		"--tag", "prop",
		"--tag", "barrel",
	}, map[string]string{"PIXELLAB_API_KEY": "test-token"})
	if code != 0 {
		t.Fatalf("expected success, got code %d stderr %s", code, stderr)
	}
}

func TestListObjectsUsesQueryParams(t *testing.T) {
	server := newAPIServer(t, func(req capturedRequest) (int, string) {
		if req.Method != http.MethodGet || req.Path != "/objects" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.Path)
		}
		if req.Query != "limit=10&offset=20" && req.Query != "offset=20&limit=10" {
			t.Fatalf("unexpected query: %q", req.Query)
		}
		return http.StatusOK, `{"objects":[]}`
	})
	defer server.Close()

	code, _, stderr := runTestCLI(t, []string{"get", "/objects", "--base-url", server.URL, "--limit", "10", "--offset", "20"}, map[string]string{"PIXELLAB_API_KEY": "test-token"})
	if code != 0 {
		t.Fatalf("expected success, got code %d stderr %s", code, stderr)
	}
}

func TestBodyJSONInlineAndFile(t *testing.T) {
	for _, tc := range []struct {
		name string
		arg  string
	}{
		{name: "inline", arg: `{"description":"inline dragon","image_size":{"width":32,"height":32}}`},
		{name: "file", arg: writeTempJSON(t, `{"description":"file dragon","image_size":{"width":64,"height":64}}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := newAPIServer(t, func(req capturedRequest) (int, string) {
				if _, ok := req.Body["description"].(string); !ok {
					t.Fatalf("body-json did not populate request body: %#v", req.Body)
				}
				return http.StatusOK, `{"ok":true}`
			})
			defer server.Close()

			code, _, stderr := runTestCLI(t, []string{"post", "/create-image-pixen", "--base-url", server.URL, "--body-json", tc.arg}, map[string]string{"PIXELLAB_API_KEY": "test-token"})
			if code != 0 {
				t.Fatalf("expected success, got code %d stderr %s", code, stderr)
			}
		})
	}
}

func TestImagePathFlagsEncodeBase64ImageObjects(t *testing.T) {
	imagePath := writeTempBytes(t, "sprite.png", []byte{0x89, 'P', 'N', 'G', 1, 2, 3})
	server := newAPIServer(t, func(req capturedRequest) (int, string) {
		image, ok := req.Body["image"].(map[string]any)
		if !ok {
			t.Fatalf("image field was not encoded as object: %#v", req.Body["image"])
		}
		assertJSONValue(t, image, "type", "base64")
		assertJSONValue(t, image, "format", "png")
		want := base64.StdEncoding.EncodeToString([]byte{0x89, 'P', 'N', 'G', 1, 2, 3})
		assertJSONValue(t, image, "base64", want)
		assertSizeObject(t, req.Body["image_size"], 7, 1)
		return http.StatusOK, `{"ok":true}`
	})
	defer server.Close()

	code, _, stderr := runTestCLI(t, []string{
		"post", "/remove-background",
		"--base-url", server.URL,
		"--image", imagePath,
		"--image-size", "7x1",
	}, map[string]string{"PIXELLAB_API_KEY": "test-token"})
	if code != 0 {
		t.Fatalf("expected success, got code %d stderr %s", code, stderr)
	}
}

func TestRepeatedImageFlagsBuildArrays(t *testing.T) {
	styleA := writeTempBytes(t, "style-a.png", []byte("a"))
	styleB := writeTempBytes(t, "style-b.png", []byte("b"))
	server := newAPIServer(t, func(req capturedRequest) (int, string) {
		images, ok := req.Body["style_images"].([]any)
		if !ok || len(images) != 2 {
			t.Fatalf("expected two style images, got %#v", req.Body["style_images"])
		}
		return http.StatusAccepted, `{"background_job_id":"job-1","status":"processing"}`
	})
	defer server.Close()

	code, _, stderr := runTestCLI(t, []string{
		"post", "/generate-with-style-v2",
		"--base-url", server.URL,
		"--style-image", styleA,
		"--style-image", styleB,
		"--description", "matching style",
		"--image-size", "64x64",
	}, map[string]string{"PIXELLAB_API_KEY": "test-token"})
	if code != 0 {
		t.Fatalf("expected success, got code %d stderr %s", code, stderr)
	}
}

func TestAsyncWaitPollsJobAndWritesImages(t *testing.T) {
	var calls int
	imageBytes := []byte("png data")
	encoded := base64.StdEncoding.EncodeToString(imageBytes)
	outDir := t.TempDir()

	server := newAPIServer(t, func(req capturedRequest) (int, string) {
		switch req.Path {
		case "/generate-image-v2":
			return http.StatusAccepted, `{"background_job_id":"job-123","status":"processing"}`
		case "/background-jobs/job-123":
			calls++
			if calls == 1 {
				return http.StatusOK, `{"status":"processing"}`
			}
			return http.StatusOK, fmt.Sprintf(`{"status":"completed","last_response":{"images":[{"base64":%q,"format":"png"}]}}`, encoded)
		default:
			t.Fatalf("unexpected path while waiting: %s", req.Path)
		}
		return http.StatusInternalServerError, `{}`
	})
	defer server.Close()

	code, _, stderr := runTestCLI(t, []string{
		"post", "/generate-image-v2",
		"--base-url", server.URL,
		"--description", "crystal sword",
		"--image-size", "32x32",
		"--wait",
		"--poll-interval", "1ms",
		"--timeout", "1s",
		"--out", outDir,
	}, map[string]string{"PIXELLAB_API_KEY": "test-token"})
	if code != 0 {
		t.Fatalf("expected success, got code %d stderr %s", code, stderr)
	}
	if calls != 2 {
		t.Fatalf("expected two job polls, got %d", calls)
	}
	files, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("read out dir: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one output image, got %d", len(files))
	}
	got, err := os.ReadFile(filepath.Join(outDir, files[0].Name()))
	if err != nil {
		t.Fatalf("read output image: %v", err)
	}
	if !bytes.Equal(got, imageBytes) {
		t.Fatalf("unexpected output bytes: %q", string(got))
	}
}

func TestAsyncWaitFindsNestedSubmissionJobID(t *testing.T) {
	var polled bool
	server := newAPIServer(t, func(req capturedRequest) (int, string) {
		switch req.Path {
		case "/objects/object-1/animations":
			return http.StatusOK, `{"submissions":[{"background_job_id":"nested-job","status":"queued"}]}`
		case "/background-jobs/nested-job":
			polled = true
			return http.StatusOK, `{"status":"completed","last_response":{"ok":true}}`
		default:
			t.Fatalf("unexpected path: %s", req.Path)
		}
		return http.StatusInternalServerError, `{}`
	})
	defer server.Close()

	code, _, stderr := runTestCLI(t, []string{
		"post", "/objects/{object_id}/animations",
		"--base-url", server.URL,
		"--object-id", "object-1",
		"--animation-description", "slash",
		"--wait",
		"--poll-interval", "1ms",
		"--timeout", "1s",
	}, map[string]string{"PIXELLAB_API_KEY": "test-token"})
	if code != 0 {
		t.Fatalf("expected success, got code %d stderr %s", code, stderr)
	}
	if !polled {
		t.Fatalf("expected nested background job to be polled")
	}
}

func TestWaitTimeoutReturnsNonZero(t *testing.T) {
	server := newAPIServer(t, func(req capturedRequest) (int, string) {
		if req.Path == "/generate-image-v2" {
			return http.StatusAccepted, `{"background_job_id":"job-timeout","status":"processing"}`
		}
		return http.StatusOK, `{"status":"processing"}`
	})
	defer server.Close()

	code, _, stderr := runTestCLI(t, []string{
		"post", "/generate-image-v2",
		"--base-url", server.URL,
		"--description", "slow job",
		"--image-size", "32x32",
		"--wait",
		"--poll-interval", "1ms",
		"--timeout", "5ms",
	}, map[string]string{"PIXELLAB_API_KEY": "test-token"})
	if code == 0 {
		t.Fatalf("expected timeout failure")
	}
	if !strings.Contains(strings.ToLower(stderr), "timeout") {
		t.Fatalf("expected timeout in stderr, got %q", stderr)
	}
}

func TestHTTPErrorReturnsNonZeroAndPrintsMessage(t *testing.T) {
	server := newAPIServer(t, func(req capturedRequest) (int, string) {
		return http.StatusUnauthorized, `{"detail":"Invalid API token"}`
	})
	defer server.Close()

	code, _, stderr := runTestCLI(t, []string{"get", "/balance", "--base-url", server.URL}, map[string]string{"PIXELLAB_API_KEY": "bad-token"})
	if code == 0 {
		t.Fatalf("expected failure exit code")
	}
	if !strings.Contains(stderr, "401") || !strings.Contains(stderr, "Invalid API token") {
		t.Fatalf("expected status and API message in stderr, got %q", stderr)
	}
}

func TestMissingRequiredFlagFailsBeforeRequest(t *testing.T) {
	called := false
	server := newAPIServer(t, func(req capturedRequest) (int, string) {
		called = true
		return http.StatusOK, `{}`
	})
	defer server.Close()

	code, _, stderr := runTestCLI(t, []string{"post", "/create-image-pixen", "--base-url", server.URL, "--image-size", "32x32"}, map[string]string{"PIXELLAB_API_KEY": "test-token"})
	if code == 0 {
		t.Fatalf("expected validation failure")
	}
	if called {
		t.Fatalf("request should not be sent when required flags are missing")
	}
	if !strings.Contains(stderr, "--description") {
		t.Fatalf("expected missing flag in stderr, got %q", stderr)
	}
}

func TestUnknownEndpointFailsBeforeRequest(t *testing.T) {
	server := newAPIServer(t, func(req capturedRequest) (int, string) {
		t.Fatalf("unknown endpoint should not send request")
		return http.StatusOK, `{}`
	})
	defer server.Close()

	code, _, stderr := runTestCLI(t, []string{"get", "/not-real", "--base-url", server.URL}, map[string]string{"PIXELLAB_API_KEY": "test-token"})
	if code == 0 {
		t.Fatalf("expected unknown endpoint failure")
	}
	if !strings.Contains(stderr, "/not-real") {
		t.Fatalf("expected endpoint path in stderr, got %q", stderr)
	}
}

func TestTokenFlagOverridesEnvironment(t *testing.T) {
	server := newAPIServer(t, func(req capturedRequest) (int, string) {
		if req.Auth != "Bearer flag-token" {
			t.Fatalf("expected flag token to win, got %q", req.Auth)
		}
		return http.StatusOK, `{"ok":true}`
	})
	defer server.Close()

	code, _, stderr := runTestCLI(t, []string{"get", "/balance", "--base-url", server.URL, "--token", "flag-token"}, map[string]string{"PIXELLAB_API_KEY": "env-token"})
	if code != 0 {
		t.Fatalf("expected success, got code %d stderr %s", code, stderr)
	}
}

func TestDotEnvPixellabAPIKeyIsUsedWhenEnvironmentIsUnset(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PIXELLAB_API_KEY=dotenv-key\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	t.Chdir(dir)

	server := newAPIServer(t, func(req capturedRequest) (int, string) {
		if req.Auth != "Bearer dotenv-key" {
			t.Fatalf("expected .env token, got %q", req.Auth)
		}
		return http.StatusOK, `{"ok":true}`
	})
	defer server.Close()

	code, _, stderr := runTestCLI(t, []string{"get", "/balance", "--base-url", server.URL}, map[string]string{})
	if code != 0 {
		t.Fatalf("expected success, got code %d stderr %s", code, stderr)
	}
}

func TestTokenPrecedenceFlagEnvironmentThenDotEnv(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PIXELLAB_API_KEY=dotenv-key\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	t.Chdir(dir)

	server := newAPIServer(t, func(req capturedRequest) (int, string) {
		if req.Auth != "Bearer env-token" {
			t.Fatalf("expected env token to override .env, got %q", req.Auth)
		}
		return http.StatusOK, `{"ok":true}`
	})
	defer server.Close()

	code, _, stderr := runTestCLI(t, []string{"get", "/balance", "--base-url", server.URL}, map[string]string{"PIXELLAB_API_KEY": "env-token"})
	if code != 0 {
		t.Fatalf("expected success, got code %d stderr %s", code, stderr)
	}
}

func TestOutputFileReceivesBinaryZipResponse(t *testing.T) {
	zipBytes := []byte("zip content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/characters/char-1/zip" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(zipBytes)
	}))
	defer server.Close()

	outPath := filepath.Join(t.TempDir(), "character.zip")
	code, _, stderr := runTestCLI(t, []string{
		"get", "/characters/{character_id}/zip",
		"--base-url", server.URL,
		"--character-id", "char-1",
		"--out", outPath,
	}, map[string]string{"PIXELLAB_API_KEY": "test-token"})
	if code != 0 {
		t.Fatalf("expected success, got code %d stderr %s", code, stderr)
	}
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read zip: %v", err)
	}
	if !bytes.Equal(got, zipBytes) {
		t.Fatalf("unexpected zip bytes: %q", string(got))
	}
}

func TestDurationFlagsAreParsed(t *testing.T) {
	start := time.Now()
	server := newAPIServer(t, func(req capturedRequest) (int, string) {
		if req.Path == "/generate-image-v2" {
			return http.StatusAccepted, `{"background_job_id":"job-duration","status":"processing"}`
		}
		return http.StatusOK, `{"status":"failed","error":"boom"}`
	})
	defer server.Close()

	code, _, _ := runTestCLI(t, []string{
		"post", "/generate-image-v2",
		"--base-url", server.URL,
		"--description", "duration test",
		"--image-size", "32x32",
		"--wait",
		"--poll-interval", "10ms",
		"--timeout", "250ms",
	}, map[string]string{"PIXELLAB_API_KEY": "test-token"})
	if code == 0 {
		t.Fatalf("expected failed job to return non-zero")
	}
	if time.Since(start) > time.Second {
		t.Fatalf("duration flags were not respected")
	}
}

func assertJSONValue(t *testing.T, object map[string]any, key string, want any) {
	t.Helper()
	if got := object[key]; got != want {
		t.Fatalf("%s: got %#v want %#v in body %#v", key, got, want, object)
	}
}

func assertSizeObject(t *testing.T, value any, width int, height int) {
	t.Helper()
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("size value is not object: %#v", value)
	}
	if object["width"] != float64(width) || object["height"] != float64(height) {
		t.Fatalf("unexpected size object: %#v", object)
	}
}

func writeTempJSON(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "body.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write json: %v", err)
	}
	return path
}

func writeTempBytes(t *testing.T, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write bytes: %v", err)
	}
	return path
}

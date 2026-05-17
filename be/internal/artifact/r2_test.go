package artifact

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type r2CapturedReq struct {
	method string
	path   string
	header http.Header
	body   []byte
}

type r2TestServer struct {
	mu       sync.Mutex
	reqs     []r2CapturedReq
	srv      *httptest.Server
	respBody []byte
}

func newR2TestServer(t *testing.T) *r2TestServer {
	t.Helper()
	ts := &r2TestServer{}
	ts.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		ts.mu.Lock()
		ts.reqs = append(ts.reqs, r2CapturedReq{
			method: r.Method,
			path:   r.URL.Path,
			header: r.Header.Clone(),
			body:   body,
		})
		ts.mu.Unlock()
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodGet && ts.respBody != nil {
			_, _ = w.Write(ts.respBody)
		}
	}))
	t.Cleanup(ts.srv.Close)
	return ts
}

func (ts *r2TestServer) last() r2CapturedReq {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.reqs[len(ts.reqs)-1]
}

func newR2ForTest(t *testing.T, ts *r2TestServer, bucket, prefix, projectID string) *r2Storage {
	t.Helper()
	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("AKID", "SECRETKEY", "")),
		config.WithRegion("auto"),
		config.WithRequestChecksumCalculation(aws.RequestChecksumCalculationWhenRequired),
		config.WithResponseChecksumValidation(aws.ResponseChecksumValidationWhenRequired),
	)
	if err != nil {
		t.Fatalf("load aws config: %v", err)
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(ts.srv.URL)
		o.UsePathStyle = true
	})
	return &r2Storage{
		client:    client,
		bucket:    bucket,
		keyPrefix: prefix + "nrflo/" + projectID + "/",
	}
}

func TestR2Storage_Put_PathAndMethod(t *testing.T) {
	t.Parallel()
	ts := newR2TestServer(t)
	const bucket = "my-bucket"
	const prefix = "myprefix/"
	const projectID = "proj-r2"
	store := newR2ForTest(t, ts, bucket, prefix, projectID)

	content := []byte("hello r2 artifact")
	if err := store.Put(context.Background(), "report.txt", bytes.NewReader(content)); err != nil {
		t.Fatalf("Put() error: %v", err)
	}

	req := ts.last()
	wantPath := "/" + bucket + "/" + prefix + "nrflo/" + projectID + "/report.txt"
	if req.path != wantPath {
		t.Errorf("PUT path = %q, want %q", req.path, wantPath)
	}
	if req.method != http.MethodPut {
		t.Errorf("method = %q, want PUT", req.method)
	}
	if !bytes.Equal(req.body, content) {
		t.Errorf("body = %q, want %q", req.body, content)
	}
}

func TestR2Storage_Put_ContentType(t *testing.T) {
	t.Parallel()
	ts := newR2TestServer(t)
	store := newR2ForTest(t, ts, "bkt", "", "p1")

	if err := store.Put(context.Background(), "doc.html", strings.NewReader("html")); err != nil {
		t.Fatalf("Put() error: %v", err)
	}
	ct := ts.last().header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want to contain text/html", ct)
	}
}

func TestR2Storage_Put_ContentTypeFallback(t *testing.T) {
	t.Parallel()
	ts := newR2TestServer(t)
	store := newR2ForTest(t, ts, "bkt", "", "p2")

	if err := store.Put(context.Background(), "unknownextension.xyz123", strings.NewReader("data")); err != nil {
		t.Fatalf("Put() error: %v", err)
	}
	ct := ts.last().header.Get("Content-Type")
	if ct != "application/octet-stream" {
		t.Errorf("Content-Type = %q, want application/octet-stream", ct)
	}
}

func TestR2Storage_Put_SigningRegionAuto(t *testing.T) {
	t.Parallel()
	ts := newR2TestServer(t)
	store := newR2ForTest(t, ts, "bkt", "", "p3")

	if err := store.Put(context.Background(), "key.txt", strings.NewReader("x")); err != nil {
		t.Fatalf("Put() error: %v", err)
	}
	auth := ts.last().header.Get("Authorization")
	if !strings.Contains(auth, "/auto/s3/aws4_request") {
		t.Errorf("Authorization header %q does not contain /auto/s3/aws4_request", auth)
	}
}

func TestR2Storage_Put_NoStreamingChecksum(t *testing.T) {
	t.Parallel()
	ts := newR2TestServer(t)
	store := newR2ForTest(t, ts, "bkt", "", "p4")

	if err := store.Put(context.Background(), "key.bin", strings.NewReader("bytes")); err != nil {
		t.Fatalf("Put() error: %v", err)
	}
	sha := ts.last().header.Get("X-Amz-Content-Sha256")
	if strings.HasPrefix(sha, "STREAMING-") {
		t.Errorf("x-amz-content-sha256 = %q, must not start with STREAMING-", sha)
	}
}

func TestR2Storage_Get_ReturnsBody(t *testing.T) {
	t.Parallel()
	ts := newR2TestServer(t)
	want := []byte("get result body")
	ts.respBody = want
	store := newR2ForTest(t, ts, "bkt", "pfx/", "p5")

	rc, err := store.Get(context.Background(), "file.txt")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("Get() body = %q, want %q", got, want)
	}
	req := ts.last()
	if req.method != http.MethodGet {
		t.Errorf("method = %q, want GET", req.method)
	}
}

func TestR2Storage_Delete_IssuesDelete(t *testing.T) {
	t.Parallel()
	ts := newR2TestServer(t)
	store := newR2ForTest(t, ts, "bkt", "", "p6")

	if err := store.Delete(context.Background(), "todelete.txt"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
	req := ts.last()
	if req.method != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", req.method)
	}
	wantPath := "/bkt/nrflo/p6/todelete.txt"
	if req.path != wantPath {
		t.Errorf("DELETE path = %q, want %q", req.path, wantPath)
	}
}

func TestNewR2_ResolvesSecretRefs(t *testing.T) {
	t.Setenv("R2_TEST_AK_7f3a", "accesskeyvalue")
	t.Setenv("R2_TEST_SK_7f3a", "secretkeyvalue")

	ts := newR2TestServer(t)
	cfg := Config{
		Mode:         ModeR2,
		AccountID:    "fake-account",
		Bucket:       "bkt",
		Prefix:       "",
		AccessKeyRef: "env:R2_TEST_AK_7f3a",
		SecretKeyRef: "env:R2_TEST_SK_7f3a",
	}
	// Override the endpoint after construction to point at test server.
	// We verify construction succeeds (refs are resolved).
	store, err := newR2(context.Background(), "proj-resolve", cfg)
	if err != nil {
		t.Fatalf("newR2() error: %v", err)
	}
	if store == nil {
		t.Fatal("newR2() returned nil")
	}
	// Confirm keyPrefix is set correctly.
	wantPrefix := "nrflo/proj-resolve/"
	if store.keyPrefix != wantPrefix {
		t.Errorf("keyPrefix = %q, want %q", store.keyPrefix, wantPrefix)
	}
	_ = ts // ts not used — we only test constructor success
}

func TestNewR2_BadSecretRef_Fails(t *testing.T) {
	t.Parallel()
	cfg := Config{
		Mode:         ModeR2,
		AccountID:    "acct",
		Bucket:       "bkt",
		AccessKeyRef: "env:UNSET_VAR_R2_AK_99z",
		SecretKeyRef: "env:UNSET_VAR_R2_SK_99z",
	}
	_, err := newR2(context.Background(), "proj", cfg)
	if err == nil {
		t.Fatal("newR2() expected error for unresolvable secret ref, got nil")
	}
}

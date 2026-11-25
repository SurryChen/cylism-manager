package githubapp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

type testTransport func(*http.Request) (*http.Response, error)

func (transport testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return transport(req)
}

func TestConvertManifest(t *testing.T) {
	client := NewClient()
	client.http.Transport = testTransport(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || req.URL.String() != "https://api.github.com/app-manifests/test-registration-code/conversions" || req.Header.Get("Authorization") != "" {
			t.Fatalf("unexpected manifest conversion request: %s %s", req.Method, req.URL)
		}
		body := `{"id":12,"slug":"cylism","pem":"generated-key","owner":{"login":"company","type":"Organization"}}`
		return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	app, err := client.ConvertManifest(context.Background(), "test-registration-code")
	if err != nil || app.ID != 12 || app.Slug != "cylism" || app.PrivateKey != "generated-key" || app.Owner.Login != "company" || app.Owner.Type != "Organization" {
		t.Fatalf("manifest conversion did not return the app: %v", err)
	}
	if _, err := client.ConvertManifest(context.Background(), "../invalid/segment"); err == nil {
		t.Fatal("accepted invalid registration code")
	}
}

func TestRepositoryTokenScopeAndAnnotatedTag(t *testing.T) {
	sha := strings.Repeat("a", 40)
	tagObject := strings.Repeat("b", 40)
	client := NewClient()
	client.http.Transport = testTransport(func(req *http.Request) (*http.Response, error) {
		var body string
		expectedToken := "Bearer repo-token"
		switch req.URL.Path {
		case "/app/installations/10/access_tokens":
			expectedToken = "Bearer app-token"
			var input struct {
				RepositoryIDs []int64           `json:"repository_ids"`
				Permissions   map[string]string `json:"permissions"`
			}
			if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}
			if req.Method != http.MethodPost || !reflect.DeepEqual(input.RepositoryIDs, []int64{99}) ||
				!reflect.DeepEqual(input.Permissions, map[string]string{"contents": "read"}) {
				t.Fatalf("unexpected installation token scope: %+v", input)
			}
			body = `{"token":"repo-token"}`
		case "/repositories/99":
			body = `{"id":99,"full_name":"org/project","private":true}`
		case "/repos/org/project/git/ref/tags/api/v1.0":
			body = `{"object":{"type":"tag","sha":"` + tagObject + `"}}`
		case "/repos/org/project/git/tags/" + tagObject:
			body = `{"object":{"type":"commit","sha":"` + sha + `"}}`
		case "/repos/org/project/commits/" + sha:
			body = `{"sha":"` + sha + `"}`
		default:
			t.Fatalf("unexpected GitHub request: %s", req.URL)
		}
		if req.Header.Get("Authorization") != expectedToken || req.URL.Host != "api.github.com" {
			t.Fatalf("unexpected GitHub destination or token for %s", req.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	ctx := context.Background()
	repository, err := client.OpenRepository(ctx, "app-token", 10, 99)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := repository.Resolve(ctx, "api/v1.0", sha)
	if err != nil || revision.Tag != "api/v1.0" || revision.SHA != sha {
		t.Fatalf("resolve annotated tag: %+v, %v", revision, err)
	}
	var invalid InvalidRevision
	if _, err := repository.Resolve(ctx, "api/v1.0", strings.Repeat("c", 40)); !errors.As(err, &invalid) {
		t.Fatalf("accepted a moved tag: %v", err)
	}
}

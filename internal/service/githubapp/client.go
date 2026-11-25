package githubapp

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	http *http.Client
	base string
}

func NewClient() *Client {
	return &Client{http: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}, base: "https://api.github.com"}
}

type App struct {
	ID   int64  `json:"id"`
	Slug string `json:"slug"`
}

type AppRegistration struct {
	App
	PrivateKey string `json:"pem"`
	Owner      struct {
		Login string `json:"login"`
		Type  string `json:"type"`
	} `json:"owner"`
}

type Installation struct {
	ID                  int64             `json:"id"`
	RepositorySelection string            `json:"repository_selection"`
	Permissions         map[string]string `json:"permissions"`
	SuspendedAt         *time.Time        `json:"suspended_at"`
	Account             struct {
		Login string `json:"login"`
		Type  string `json:"type"`
	} `json:"account"`
}

type Repository struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
	Private  bool   `json:"private"`
}

type Revision struct {
	Tag     string `json:"tag,omitempty"`
	SHA     string `json:"sha"`
	Message string `json:"message,omitempty"`
}

type HTTPError int
type RequestError string

func (e RequestError) Error() string { return string(e) }

type InvalidRevision string

func (e InvalidRevision) Error() string { return string(e) }

func (e HTTPError) Error() string {
	switch int(e) {
	case http.StatusUnauthorized:
		return "GitHub App 凭据无效或已撤销"
	case http.StatusForbidden, http.StatusTooManyRequests:
		return "GitHub 拒绝访问，请检查安装权限或稍后重试"
	case http.StatusNotFound:
		return "GitHub 资源不存在，或安装未授权访问"
	case http.StatusUnprocessableEntity:
		return "GitHub 无法使用所选安装权限访问这个仓库"
	default:
		return fmt.Sprintf("GitHub 返回 HTTP %d", int(e))
	}
}

func AppToken(appID int64, privateKey string) (string, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(privateKey)))
	if appID <= 0 || block == nil {
		return "", errors.New("GitHub App ID 或 PEM 私钥无效")
	}
	var key *rsa.PrivateKey
	switch block.Type {
	case "RSA PRIVATE KEY":
		key, _ = x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		parsed, _ := x509.ParsePKCS8PrivateKey(block.Bytes)
		key, _ = parsed.(*rsa.PrivateKey)
	}
	if key == nil || key.N.BitLen() < 2048 {
		return "", errors.New("请输入 GitHub App 生成的 RSA 私钥")
	}
	now := time.Now()
	claims, err := json.Marshal(struct {
		Issuer    string `json:"iss"`
		IssuedAt  int64  `json:"iat"`
		ExpiresAt int64  `json:"exp"`
	}{strconv.FormatInt(appID, 10), now.Add(-time.Minute).Unix(), now.Add(8 * time.Minute).Unix()})
	if err != nil {
		return "", err
	}
	encode := base64.RawURLEncoding.EncodeToString
	message := encode([]byte(`{"alg":"RS256","typ":"JWT"}`)) + "." + encode(claims)
	hash := sha256.Sum256([]byte(message))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	if err != nil {
		return "", err
	}
	return message + "." + encode(signature), nil
}

func (c *Client) request(ctx context.Context, method, path, token string, input, output any) (bool, error) {
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return false, err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return false, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return false, RequestError("无法连接 GitHub API")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		if res.StatusCode == http.StatusForbidden && res.Header.Get("X-RateLimit-Remaining") == "0" {
			return false, HTTPError(http.StatusTooManyRequests)
		}
		return false, HTTPError(res.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(output); err != nil {
		return false, RequestError("无法读取 GitHub API 响应")
	}
	// Construct the next request locally. GitHub's Link header never supplies
	// the destination for an authenticated request.
	return strings.Contains(res.Header.Get("Link"), `rel="next"`), nil
}

var manifestCode = regexp.MustCompile(`^[A-Za-z0-9_-]{16,256}$`)

func (c *Client) ConvertManifest(ctx context.Context, code string) (AppRegistration, error) {
	var app AppRegistration
	if !manifestCode.MatchString(code) {
		return app, RequestError("GitHub 注册代码无效")
	}
	_, err := c.request(ctx, http.MethodPost, "/app-manifests/"+code+"/conversions", "", nil, &app)
	return app, err
}

func pageQuery(page int) string {
	return "?per_page=50&page=" + strconv.Itoa(page)
}

func (c *Client) Installations(ctx context.Context, token string, page int) ([]Installation, bool, error) {
	items := []Installation{}
	more, err := c.request(ctx, http.MethodGet, "/app/installations"+pageQuery(page), token, nil, &items)
	return items, more, err
}

func (c *Client) installationToken(ctx context.Context, appToken string, installationID, repoID int64) (string, error) {
	payload := map[string]any{"permissions": map[string]string{"metadata": "read"}}
	if repoID > 0 {
		payload["repository_ids"] = []int64{repoID}
		payload["permissions"] = map[string]string{"contents": "read"}
	}
	var result struct {
		Token string `json:"token"`
	}
	_, err := c.request(ctx, http.MethodPost, "/app/installations/"+strconv.FormatInt(installationID, 10)+"/access_tokens", appToken, payload, &result)
	if err == nil && result.Token == "" {
		err = RequestError("GitHub 未返回安装令牌")
	}
	return result.Token, err
}

func (c *Client) Repositories(ctx context.Context, appToken string, installationID int64, page int) ([]Repository, bool, error) {
	token, err := c.installationToken(ctx, appToken, installationID, 0)
	if err != nil {
		return nil, false, err
	}
	result := struct {
		Repositories []Repository `json:"repositories"`
	}{Repositories: []Repository{}}
	next, err := c.request(ctx, http.MethodGet, "/installation/repositories"+pageQuery(page), token, nil, &result)
	return result.Repositories, next, err
}

var validRepository = regexp.MustCompile(`^[A-Za-z0-9-]+/[A-Za-z0-9_.-]+$`)

type RepositorySession struct {
	client     *Client
	token      string
	Repository Repository
}

func (c *Client) OpenRepository(ctx context.Context, appToken string, installationID, repoID int64) (*RepositorySession, error) {
	token, err := c.installationToken(ctx, appToken, installationID, repoID)
	if err != nil {
		return nil, err
	}
	var repository Repository
	_, err = c.request(ctx, http.MethodGet, "/repositories/"+strconv.FormatInt(repoID, 10), token, nil, &repository)
	if err != nil {
		return nil, err
	}
	if repository.ID != repoID || !validRepository.MatchString(repository.FullName) || strings.HasSuffix(repository.FullName, "/.") || strings.HasSuffix(repository.FullName, "/..") {
		return nil, RequestError("GitHub 返回了无效的仓库信息")
	}
	return &RepositorySession{client: c, token: token, Repository: repository}, nil
}

var SHA = regexp.MustCompile(`^[a-fA-F0-9]{40}$`)
var commitRef = regexp.MustCompile(`^[a-fA-F0-9]{7,40}$`)

func (r *RepositorySession) read(ctx context.Context, path string, output any) (bool, error) {
	return r.client.request(ctx, http.MethodGet, "/repos/"+r.Repository.FullName+path, r.token, nil, output)
}

type commit struct {
	SHA    string `json:"sha"`
	Commit struct {
		Message string `json:"message"`
	} `json:"commit"`
}

func (r *RepositorySession) Revisions(ctx context.Context, kind string, page int) ([]Revision, bool, error) {
	items := []Revision{}
	if kind == "tags" {
		var tags []struct {
			Name   string `json:"name"`
			Commit commit `json:"commit"`
		}
		more, err := r.read(ctx, "/tags"+pageQuery(page), &tags)
		for _, tag := range tags {
			if SHA.MatchString(tag.Commit.SHA) {
				items = append(items, Revision{Tag: tag.Name, SHA: tag.Commit.SHA})
			}
		}
		return items, more, err
	}
	var commits []commit
	more, err := r.read(ctx, "/commits"+pageQuery(page), &commits)
	for _, item := range commits {
		if SHA.MatchString(item.SHA) {
			message, _, _ := strings.Cut(item.Commit.Message, "\n")
			items = append(items, Revision{SHA: item.SHA, Message: message})
		}
	}
	return items, more, err
}

func (r *RepositorySession) Resolve(ctx context.Context, tag, sha string) (Revision, error) {
	expected := sha
	if tag != "" {
		segments := strings.Split(tag, "/")
		for i, part := range segments {
			if part == "" || part == "." || part == ".." {
				return Revision{}, InvalidRevision("GitHub Tag 无效")
			}
			segments[i] = url.PathEscape(part)
		}
		var ref struct {
			Object gitObject `json:"object"`
		}
		if _, err := r.read(ctx, "/git/ref/tags/"+strings.Join(segments, "/"), &ref); err != nil {
			return Revision{}, err
		}
		for depth := 0; ref.Object.Type == "tag" && depth < 8; depth++ {
			if !SHA.MatchString(ref.Object.SHA) {
				return Revision{}, RequestError("GitHub Tag 对象无效")
			}
			objectID := ref.Object.SHA
			if _, err := r.read(ctx, "/git/tags/"+objectID, &ref); err != nil {
				return Revision{}, err
			}
		}
		if ref.Object.Type != "commit" || !SHA.MatchString(ref.Object.SHA) {
			return Revision{}, InvalidRevision("GitHub Tag 未指向提交")
		}
		sha = ref.Object.SHA
	}
	if !commitRef.MatchString(sha) {
		return Revision{}, InvalidRevision("请输入至少 7 位十六进制的提交 SHA")
	}
	var resolved commit
	if _, err := r.read(ctx, "/commits/"+sha, &resolved); err != nil {
		return Revision{}, err
	}
	if !SHA.MatchString(resolved.SHA) || !strings.HasPrefix(strings.ToLower(resolved.SHA), strings.ToLower(sha)) {
		return Revision{}, RequestError("GitHub 返回的提交与请求不一致")
	}
	if expected != "" && !strings.HasPrefix(strings.ToLower(resolved.SHA), strings.ToLower(expected)) {
		return Revision{}, InvalidRevision("GitHub Tag 指向已变更，请重新选择版本")
	}
	return Revision{Tag: tag, SHA: strings.ToLower(resolved.SHA)}, nil
}

type gitObject struct {
	Type string `json:"type"`
	SHA  string `json:"sha"`
}

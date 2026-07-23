package nginx

import (
	"strings"
	"testing"
)

func TestRenderHTTP(t *testing.T) {
	data := &TemplateData{
		Domain:   "example.com",
		Port:     80,
		RootPath: "/var/www/example",
	}
	conf, err := RenderHTTP(data)
	if err != nil {
		t.Fatalf("RenderHTTP: %v", err)
	}
	if !strings.Contains(conf, "server_name example.com") {
		t.Error("missing server_name")
	}
	if !strings.Contains(conf, "listen 80") {
		t.Error("missing listen 80")
	}
	if !strings.Contains(conf, "root /var/www/example") {
		t.Error("missing root")
	}
	if !strings.Contains(conf, "# Managed by Cylism Manager") {
		t.Error("missing managed comment")
	}
}

func TestRenderHTTPWithUpstream(t *testing.T) {
	data := &TemplateData{
		Domain:   "api.example.com",
		Port:     80,
		RootPath: "/var/www/api",
		Upstream: "http://127.0.0.1:3000",
	}
	conf, err := RenderHTTP(data)
	if err != nil {
		t.Fatalf("RenderHTTP: %v", err)
	}
	if !strings.Contains(conf, "proxy_pass http://127.0.0.1:3000") {
		t.Error("missing proxy_pass")
	}
}

func TestRenderHTTPS(t *testing.T) {
	data := &TemplateData{
		Domain:   "secure.example.com",
		Port:     443,
		RootPath: "/var/www/secure",
		CertPath: "/etc/ssl/secure.pem",
		KeyPath:  "/etc/ssl/secure.key",
	}
	conf, err := RenderHTTPS(data)
	if err != nil {
		t.Fatalf("RenderHTTPS: %v", err)
	}
	if !strings.Contains(conf, "listen 443 ssl") {
		t.Error("missing listen 443 ssl")
	}
	if !strings.Contains(conf, "ssl_certificate /etc/ssl/secure.pem") {
		t.Error("missing ssl_certificate")
	}
	if !strings.Contains(conf, "return 301 https") {
		t.Error("missing HTTP→HTTPS redirect")
	}
}

func TestConfPath(t *testing.T) {
	path := ConfPath("example.com")
	if path != "/etc/nginx/conf.d/example.com.conf" {
		t.Errorf("expected /etc/nginx/conf.d/example.com.conf, got %s", path)
	}
}

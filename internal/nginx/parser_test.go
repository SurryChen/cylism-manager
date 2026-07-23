package nginx

import (
	"testing"
)

func TestParseServerBlockHTTP(t *testing.T) {
	block := `server {
    listen 80;
    server_name example.com www.example.com;
    root /var/www/example;
    location / {
        try_files $uri $uri/ =404;
    }
}`
	s := parseServerBlock(block)
	if s.Domain != "example.com" {
		t.Errorf("expected domain 'example.com', got '%s'", s.Domain)
	}
	if s.RootPath != "/var/www/example" {
		t.Errorf("expected root '/var/www/example', got '%s'", s.RootPath)
	}
	if s.SSLEnabled {
		t.Error("expected SSLEnabled=false")
	}
}

func TestParseServerBlockHTTPS(t *testing.T) {
	block := `server {
    listen 443 ssl http2;
    server_name secure.example.com;
    root /var/www/secure;
    ssl_certificate /etc/ssl/cert.pem;
    ssl_certificate_key /etc/ssl/key.pem;
}`
	s := parseServerBlock(block)
	if !s.SSLEnabled {
		t.Error("expected SSLEnabled=true")
	}
	if s.CertPath != "/etc/ssl/cert.pem" {
		t.Errorf("expected cert path, got '%s'", s.CertPath)
	}
	if s.KeyPath != "/etc/ssl/key.pem" {
		t.Errorf("expected key path, got '%s'", s.KeyPath)
	}
}

func TestParseServerBlockProxyPass(t *testing.T) {
	block := `server {
    listen 80;
    server_name api.example.com;
    location / {
        proxy_pass http://127.0.0.1:3000;
    }
}`
	s := parseServerBlock(block)
	if s.ProxyPass != "http://127.0.0.1:3000" {
		t.Errorf("expected proxy_pass, got '%s'", s.ProxyPass)
	}
}

func TestParseNginxConfig(t *testing.T) {
	config := `server {
    listen 80;
    server_name site1.com;
    root /var/www/site1;
}
server {
    listen 443 ssl;
    server_name site2.com;
    ssl_certificate /etc/ssl/site2.pem;
    ssl_certificate_key /etc/ssl/site2.key;
}`

	servers := ParseNginxConfig(config)
	if len(servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(servers))
	}
	if servers[0].Domain != "site1.com" {
		t.Errorf("expected site1.com, got '%s'", servers[0].Domain)
	}
	if servers[1].Domain != "site2.com" {
		t.Errorf("expected site2.com, got '%s'", servers[1].Domain)
	}
}

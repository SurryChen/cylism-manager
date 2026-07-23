package nginx

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/cylism/cylism-manager/internal/model"
)

const httpTemplate = `# Managed by Cylism Manager - DO NOT EDIT manually
server {
    listen {{.Port}};
    server_name {{.Domain}};
    root {{.RootPath}};

    access_log /var/log/nginx/{{.Domain}}.access.log;
    error_log /var/log/nginx/{{.Domain}}.error.log;

    {{if .Upstream}}location / {
        proxy_pass {{.Upstream}};
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
    {{else}}location / {
        try_files $uri $uri/ =404;
    }
    {{end}}
{{range .ExtraLocations}}
    location {{.Path}} {
        {{if .ProxyPass}}proxy_pass {{.ProxyPass}};{{end}}
        {{if .Root}}root {{.Root}};{{end}}
        {{if .Extra}}{{.Extra}}{{end}}
    }
{{end}}
}
`

const httpsTemplate = `# Managed by Cylism Manager - DO NOT EDIT manually
# HTTP → HTTPS redirect
server {
    listen 80;
    server_name {{.Domain}};
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl http2;
    server_name {{.Domain}};
    root {{.RootPath}};

    ssl_certificate {{.CertPath}};
    ssl_certificate_key {{.KeyPath}};

    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256;
    ssl_prefer_server_ciphers on;

    access_log /var/log/nginx/{{.Domain}}.access.log;
    error_log /var/log/nginx/{{.Domain}}.error.log;

    {{if .Upstream}}location / {
        proxy_pass {{.Upstream}};
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
    {{else}}location / {
        try_files $uri $uri/ =404;
    }
    {{end}}
{{range .ExtraLocations}}
    location {{.Path}} {
        {{if .ProxyPass}}proxy_pass {{.ProxyPass}};{{end}}
        {{if .Root}}root {{.Root}};{{end}}
        {{if .Extra}}{{.Extra}}{{end}}
    }
{{end}}
}
`

// TemplateData 模板渲染数据
type TemplateData struct {
	Domain         string
	Port           int
	RootPath       string
	Upstream       string
	CertPath       string
	KeyPath        string
	ExtraLocations []model.LocationConfig
}

// RenderHTTP 渲染纯 HTTP server 块
func RenderHTTP(data *TemplateData) (string, error) {
	tmpl, err := template.New("http").Parse(httpTemplate)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	return buf.String(), nil
}

// RenderHTTPS 渲染 HTTPS server 块（含 HTTP 重定向）
func RenderHTTPS(data *TemplateData) (string, error) {
	tmpl, err := template.New("https").Parse(httpsTemplate)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	return buf.String(), nil
}

// ConfPath 返回站点 NGINX 配置文件的路径
func ConfPath(domain string) string {
	return fmt.Sprintf("/etc/nginx/conf.d/%s.conf", domain)
}

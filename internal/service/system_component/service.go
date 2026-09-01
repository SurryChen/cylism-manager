package system_component

import (
	"fmt"
	"sigs.k8s.io/yaml"
	"strings"
	"time"
)

var whitelist = map[string]string{"coredns": "kube-system", "traefik": "kube-system", "metrics-server": "kube-system", "local-path-provisioner": "kube-system", "servicelb": "kube-system"}

func Namespace(chart string) (string, bool) {
	ns, ok := whitelist[strings.TrimSpace(chart)]
	return ns, ok
}
func Charts() map[string]string {
	out := make(map[string]string, len(whitelist))
	for k, v := range whitelist {
		out[k] = v
	}
	return out
}
func ValidateReadTimeout(value string) error {
	v := strings.TrimSpace(value)
	if v == "" {
		return fmt.Errorf("超时时间不能为空")
	}
	if len(v) > 16 || strings.ContainsAny(v, "\r\n") {
		return fmt.Errorf("超时时间格式无效")
	}
	return nil
}

const defaultReadTimeout = "60s"

func NormalizeReadTimeout(value string) (string, error) {
	v := strings.TrimSpace(value)
	if v == "" {
		return "", nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < time.Minute || d > time.Hour {
		return "", fmt.Errorf("Traefik 读取超时必须在 1 分钟至 1 小时之间")
	}
	return v, nil
}
func ConfiguredReadTimeout(valuesContent string) string {
	var values struct {
		AdditionalArguments []string `json:"additionalArguments"`
	}
	if yaml.Unmarshal([]byte(valuesContent), &values) != nil {
		return ""
	}
	web, secure := "", ""
	for _, arg := range values.AdditionalArguments {
		if strings.HasPrefix(arg, "--entryPoints.web.transport.respondingTimeouts.readTimeout=") {
			web = strings.TrimPrefix(arg, "--entryPoints.web.transport.respondingTimeouts.readTimeout=")
		}
		if strings.HasPrefix(arg, "--entryPoints.websecure.transport.respondingTimeouts.readTimeout=") {
			secure = strings.TrimPrefix(arg, "--entryPoints.websecure.transport.respondingTimeouts.readTimeout=")
		}
	}
	if web == "" || web != secure {
		return ""
	}
	return web
}

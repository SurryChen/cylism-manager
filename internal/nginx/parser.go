package nginx

import (
	"regexp"
	"strings"
)

// ParsedServer 解析后的 server 块信息
type ParsedServer struct {
	Domain       string
	Port         int
	SSLEnabled   bool
	RootPath     string
	CertPath     string
	KeyPath      string
	ProxyPass    string
	ServerBlock  string // 原始 server{} 块文本
}

// ParseNginxConfig 解析 nginx -T 输出，提取所有 server{} 块
func ParseNginxConfig(config string) []ParsedServer {
	var servers []ParsedServer

	// 提取所有 server { ... } 块
	blocks := extractServerBlocks(config)
	for _, block := range blocks {
		server := parseServerBlock(block)
		if server.Domain != "" {
			server.ServerBlock = block
			servers = append(servers, server)
		}
	}
	return servers
}

// extractServerBlocks 提取 server { ... } 块
func extractServerBlocks(config string) []string {
	var blocks []string

	// 简化解析器：找到每个 "server" 关键字和对应的 { }
	re := regexp.MustCompile(`server\s*\{`)
	locs := re.FindAllStringIndex(config, -1)

	for i, loc := range locs {
		start := loc[1] - 1 // '{' 的位置
		depth := 0
		end := -1
		for j := start; j < len(config); j++ {
			if config[j] == '{' {
				depth++
			} else if config[j] == '}' {
				depth--
				if depth == 0 {
					end = j + 1
					break
				}
			}
		}
		if end > 0 {
			blockEnd := end
			// 下一个 server 块之前或字符串结尾
			if i+1 < len(locs) {
				blockEnd = locs[i+1][0]
			}
			blocks = append(blocks, config[locs[i][0]:blockEnd])
		}
	}
	return blocks
}

// parseServerBlock 解析单个 server 块
func parseServerBlock(block string) ParsedServer {
	s := ParsedServer{Port: 80}

	// listen 指令
	listenRe := regexp.MustCompile(`listen\s+(\d+)(?:\s+ssl)?;`)
	if m := listenRe.FindStringSubmatch(block); len(m) > 1 {
		// 检查是否有 ssl 关键字
		if strings.Contains(block, "ssl") && strings.Contains(m[0], "443") {
			s.Port = 443
		}
	}

	// ssl 检测
	if strings.Contains(block, "ssl_certificate") {
		s.SSLEnabled = true
	}

	// server_name
	snRe := regexp.MustCompile(`server_name\s+([^;]+);`)
	if m := snRe.FindStringSubmatch(block); len(m) > 1 {
		s.Domain = strings.TrimSpace(strings.Split(m[1], " ")[0])
	}

	// root
	rootRe := regexp.MustCompile(`root\s+([^;]+);`)
	if m := rootRe.FindStringSubmatch(block); len(m) > 1 {
		s.RootPath = strings.TrimSpace(m[1])
	}

	// ssl_certificate
	certRe := regexp.MustCompile(`ssl_certificate\s+([^;]+);`)
	if m := certRe.FindStringSubmatch(block); len(m) > 1 {
		s.CertPath = strings.TrimSpace(m[1])
	}

	// ssl_certificate_key
	keyRe := regexp.MustCompile(`ssl_certificate_key\s+([^;]+);`)
	if m := keyRe.FindStringSubmatch(block); len(m) > 1 {
		s.KeyPath = strings.TrimSpace(m[1])
	}

	// proxy_pass
	ppRe := regexp.MustCompile(`proxy_pass\s+([^;]+);`)
	if m := ppRe.FindStringSubmatch(block); len(m) > 1 {
		s.ProxyPass = strings.TrimSpace(m[1])
	}

	return s
}

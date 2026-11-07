package database

import (
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ConnectionHint contains metadata only. Never retain a raw DSN or password in
// a hint: callers may return these values to the browser.
type ConnectionHint struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Database string `json:"database_name"`
	Username string `json:"username"`
	Source   string `json:"source"`
}

// DetectConnections understands MySQL URLs, Go TCP DSNs, grouped environment
// variables and YAML/JSON/dotenv files. It does not execute shell expressions,
// resolve arbitrary DNS aliases or infer a connection from a password alone.
func DetectConnections(values map[string]string, source string) []ConnectionHint {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	groups := map[string]map[string]string{}
	var hints []ConnectionHint
	for _, key := range keys {
		value := strings.TrimSpace(values[key])
		if hint, ok := parseMySQLAddress(value); ok {
			hint.Source = source + key
			hints = append(hints, hint)
		}
		name := strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
		for _, suffix := range []string{"HOST", "PORT", "DATABASE", "DBNAME", "NAME", "USERNAME", "USER"} {
			prefix, ok := strings.CutSuffix(name, "_"+suffix)
			if !ok || !(prefix == "DB" || strings.Contains(prefix, "MYSQL") || strings.Contains(prefix, "DATABASE") || strings.HasSuffix(prefix, "_DB")) {
				continue
			}
			if groups[prefix] == nil {
				groups[prefix] = map[string]string{}
			}
			groups[prefix][suffix] = value
			break
		}
	}
	groupNames := make([]string, 0, len(groups))
	for name := range groups {
		groupNames = append(groupNames, name)
	}
	sort.Strings(groupNames)
	for _, name := range groupNames {
		group := groups[name]
		if group["HOST"] == "" {
			continue
		}
		hint := ConnectionHint{Host: group["HOST"], Port: 3306, Database: firstValue(group, "DATABASE", "DBNAME", "NAME"), Username: firstValue(group, "USERNAME", "USER"), Source: source + name + "_*"}
		if port := group["PORT"]; port != "" {
			hint.Port, _ = strconv.Atoi(port)
		} else if host, port, err := net.SplitHostPort(hint.Host); err == nil {
			hint.Host = host
			hint.Port, _ = strconv.Atoi(port)
		}
		if validHint(hint) {
			hints = append(hints, hint)
		}
	}
	return hints
}

func firstValue(values map[string]string, names ...string) string {
	for _, name := range names {
		if values[name] != "" {
			return values[name]
		}
	}
	return ""
}

func validHint(hint ConnectionHint) bool {
	return hint.Host != "" && hint.Port > 0 && hint.Port <= 65535 && !strings.ContainsAny(hint.Host, "$\n\r\t /@") && !strings.ContainsAny(hint.Database+hint.Username, "$\n\r")
}

func parseMySQLAddress(value string) (ConnectionHint, bool) {
	hint := ConnectionHint{Port: 3306}
	if strings.HasPrefix(value, "mysql://") || strings.HasPrefix(value, "jdbc:mysql://") {
		parsed, err := url.Parse(strings.TrimPrefix(value, "jdbc:"))
		if err != nil {
			return hint, false
		}
		hint.Host, hint.Database = parsed.Hostname(), strings.TrimPrefix(parsed.Path, "/")
		if parsed.User != nil {
			hint.Username = parsed.User.Username()
		}
		if parsed.Port() != "" {
			hint.Port, _ = strconv.Atoi(parsed.Port())
		}
	} else {
		// Split at the final network marker; passwords may contain '@' or ':'.
		index := strings.LastIndex(value, "@tcp(")
		if index < 0 {
			return hint, false
		}
		address, database, ok := strings.Cut(value[index+5:], ")/")
		if !ok {
			return hint, false
		}
		hint.Host = address
		if host, port, err := net.SplitHostPort(address); err == nil {
			hint.Host = host
			hint.Port, _ = strconv.Atoi(port)
		}
		hint.Database, _, _ = strings.Cut(database, "?")
		hint.Database, _ = url.PathUnescape(hint.Database)
		hint.Username, _, _ = strings.Cut(value[:index], ":")
	}
	return hint, validHint(hint)
}

// DetectFileConnections parses structure rather than searching arbitrary text
// for host substrings. A YAML node traversal avoids following alias cycles.
func DetectFileConnections(content, source string) []ConnectionHint {
	if len(content) > 1024*1024 {
		return nil
	}
	var doc yaml.Node
	var hints []ConnectionHint
	if yaml.Unmarshal([]byte(content), &doc) == nil {
		var walk func(*yaml.Node, string, int)
		walk = func(node *yaml.Node, path string, depth int) {
			if depth > 24 {
				return
			}
			if node.Kind == yaml.MappingNode {
				values := map[string]string{}
				for i := 0; i+1 < len(node.Content); i += 2 {
					key, value := node.Content[i].Value, node.Content[i+1]
					if value.Kind == yaml.ScalarNode {
						values[key] = value.Value
						// Nested mysql/database sections commonly use bare host/name.
						values[strings.ReplaceAll(path, ".", "_")+"_"+key] = value.Value
					} else {
						walk(value, path+"."+key, depth+1)
					}
				}
				hints = append(hints, DetectConnections(values, source+path+":")...)
			} else if node.Kind == yaml.DocumentNode || node.Kind == yaml.SequenceNode {
				for _, child := range node.Content {
					walk(child, path, depth+1)
				}
			}
		}
		walk(&doc, "", 0)
	}
	// dotenv is not YAML; parse assignments without invoking a shell.
	values := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		key, value, ok := strings.Cut(strings.TrimPrefix(strings.TrimSpace(line), "export "), "=")
		if !ok || strings.ContainsAny(key, " \t:#") {
			continue
		}
		values[key] = strings.Trim(strings.TrimSpace(value), "\"'")
	}
	return append(hints, DetectConnections(values, source+":")...)
}

func NormalizeDatabaseHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}

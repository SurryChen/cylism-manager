package logging

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type Stream struct {
	Labels map[string]string
	Values [][]string
}
type Response struct{ Streams []Stream }
type Line struct {
	Timestamp string
	Line      string
	Labels    map[string]string
}

func NormalizeLines(responses []Response, limit int) []Line {
	if limit < 1 {
		limit = DefaultLimit
	}
	lines := make([]Line, 0)
	seen := make(map[string]struct{})
	for _, response := range responses {
		for _, stream := range response.Streams {
			for _, value := range stream.Values {
				if len(value) < 2 {
					continue
				}
				labels := make(map[string]string, len(stream.Labels))
				for k, v := range stream.Labels {
					labels[k] = v
				}
				timestamp := formatTimestamp(value[0])
				serialized, _ := json.Marshal(labels)
				key := timestamp + "\x00" + string(serialized) + "\x00" + value[1]
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				lines = append(lines, Line{Timestamp: timestamp, Line: value[1], Labels: labels})
			}
		}
	}
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].Timestamp > lines[j].Timestamp })
	if len(lines) > limit {
		return lines[:limit]
	}
	return lines
}
func formatTimestamp(raw string) string {
	var n int64
	if _, err := fmt.Sscan(raw, &n); err != nil {
		return raw
	}
	return time.Unix(0, n).UTC().Format(time.RFC3339Nano)
}

const (
	MaxRange       = 24 * time.Hour
	MaxLimit       = 500
	DefaultLimit   = 200
	MaxKeywordSize = 256
	MaxQueryTerms  = 16
	MaxQueryGroups = 8
)

var Ranges = map[string]time.Duration{"1h": time.Hour, "6h": 6 * time.Hour, "24h": 24 * time.Hour}

type QueryRequest struct {
	ApplicationID, EnvironmentID, ProjectID                                       uint
	Range, Keyword, Namespace, Pod, Container, Node, Workload, StartTime, EndTime string
	RawLogQL                                                                      string
	Limit                                                                         int
}

func ValidateQuery(r *QueryRequest) error {
	r.Range = strings.TrimSpace(r.Range)
	if r.Range == "" {
		r.Range = "1h"
	}
	if r.Range != "custom" {
		if d, ok := Ranges[r.Range]; !ok || d > MaxRange {
			return fmt.Errorf("日志时间范围最多支持 24 小时")
		}
	}
	if strings.TrimSpace(r.RawLogQL) != "" {
		return fmt.Errorf("日志查询仅支持平台提供的筛选条件，不能提交原始 LogQL")
	}
	r.Keyword = strings.TrimSpace(r.Keyword)
	if len(r.Keyword) > MaxKeywordSize || strings.ContainsAny(r.Keyword, "\r\n") {
		return fmt.Errorf("日志关键字不能超过 256 个字符且不能包含换行")
	}
	if r.Limit == 0 {
		r.Limit = DefaultLimit
	}
	if r.Limit < 1 || r.Limit > MaxLimit {
		return fmt.Errorf("单次日志查询最多返回 500 行")
	}
	for _, v := range []*string{&r.Namespace, &r.Pod, &r.Container, &r.Node, &r.Workload} {
		*v = strings.TrimSpace(*v)
		if len(*v) > 253 || strings.ContainsAny(*v, "\r\n") {
			return fmt.Errorf("日志筛选值格式无效")
		}
	}
	r.StartTime, r.EndTime = strings.TrimSpace(r.StartTime), strings.TrimSpace(r.EndTime)
	return nil
}

func ResolveBounds(r QueryRequest, now time.Time) (time.Time, time.Time, error) {
	if r.StartTime != "" || r.EndTime != "" {
		if r.StartTime == "" || r.EndTime == "" {
			return time.Time{}, time.Time{}, fmt.Errorf("精确时间范围必须同时填写开始和结束时间")
		}
		start, err := time.Parse(time.RFC3339Nano, r.StartTime)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("开始时间格式无效")
		}
		end, err := time.Parse(time.RFC3339Nano, r.EndTime)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("结束时间格式无效")
		}
		if !end.After(start) {
			return time.Time{}, time.Time{}, fmt.Errorf("结束时间必须晚于开始时间")
		}
		if end.Sub(start) > MaxRange {
			return time.Time{}, time.Time{}, fmt.Errorf("日志时间范围最多支持 24 小时")
		}
		return start.UTC(), end.UTC(), nil
	}
	if r.Range == "custom" {
		return time.Time{}, time.Time{}, fmt.Errorf("精确时间范围必须同时填写开始和结束时间")
	}
	return now.Add(-Ranges[r.Range]), now, nil
}

func BuildLogQLQueries(r QueryRequest) ([]string, error) {
	labels := make([]string, 0, 5)
	for _, f := range []struct{ label, value string }{{"namespace", r.Namespace}, {"pod", r.Pod}, {"container", r.Container}, {"node", r.Node}, {"workload", r.Workload}} {
		if f.value != "" {
			labels = append(labels, f.label+`="`+escape(f.value)+`"`)
		}
	}
	if len(labels) == 0 {
		labels = append(labels, `namespace=~".+"`)
	}
	branches, err := parseExpression(r.Keyword)
	if err != nil {
		return nil, err
	}
	queries := make([]string, 0, len(branches))
	selector := "{" + strings.Join(labels, ",") + "}"
	for _, branch := range branches {
		q := selector
		for _, term := range branch {
			q += ` |= "` + escape(term) + `"`
		}
		queries = append(queries, q)
	}
	return queries, nil
}

func escape(v string) string { return strings.NewReplacer("\\", "\\\\", `"`, `\"`).Replace(v) }

func parseExpression(raw string) ([][]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return [][]string{{}}, nil
	}
	branches := make([][]string, 1)
	expect := true
	terms := 0
	hasOp := false
	for i := 0; i < len(raw); {
		for i < len(raw) && (raw[i] == ' ' || raw[i] == '\t') {
			i++
		}
		if i == len(raw) {
			break
		}
		term := ""
		quoted := raw[i] == '"'
		if quoted {
			i++
			for i < len(raw) && raw[i] != '"' {
				if raw[i] == '\\' && i+1 < len(raw) {
					i++
					term += string(raw[i])
					i++
					continue
				}
				term += string(raw[i])
				i++
			}
			if i >= len(raw) {
				return nil, fmt.Errorf("日志表达式中的字符串缺少结束引号")
			}
			i++
		} else {
			start := i
			for i < len(raw) && raw[i] != ' ' && raw[i] != '\t' {
				i++
			}
			term = raw[start:i]
		}
		if !quoted && strings.EqualFold(term, "AND") {
			if expect {
				return nil, fmt.Errorf("日志表达式缺少关键字")
			}
			expect = true
			hasOp = true
			continue
		}
		if !quoted && strings.EqualFold(term, "OR") {
			if expect {
				return nil, fmt.Errorf("日志表达式缺少关键字")
			}
			if len(branches) >= MaxQueryGroups {
				return nil, fmt.Errorf("日志表达式最多支持 %d 个 OR 分支", MaxQueryGroups)
			}
			branches = append(branches, nil)
			expect = true
			hasOp = true
			continue
		}
		if strings.ContainsAny(term, "()") {
			return nil, fmt.Errorf("日志表达式暂不支持括号")
		}
		terms++
		if terms > MaxQueryTerms {
			return nil, fmt.Errorf("日志表达式最多支持 %d 个关键词", MaxQueryTerms)
		}
		branches[len(branches)-1] = append(branches[len(branches)-1], term)
		expect = false
	}
	if expect {
		return nil, fmt.Errorf("日志表达式不能以 AND 或 OR 结束")
	}
	if !hasOp {
		return [][]string{{raw}}, nil
	}
	return branches, nil
}

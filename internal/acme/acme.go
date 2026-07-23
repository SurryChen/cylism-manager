package acme

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// IssueRequest 签发证书请求
type IssueRequest struct {
	Domain      string
	Challenge   string // http / dns
	Webroot     string
	DNSProvider string
}

// IssueResult 签发结果
type IssueResult struct {
	Success      bool
	CertPath     string
	KeyPath      string
	FullchainPath string
	Output       string
	Error        string
}

// Issue 签发证书
func Issue(req *IssueRequest) (*IssueResult, error) {
	args := []string{"--issue", "-d", req.Domain, "--force"}
	switch req.Challenge {
	case "http":
		if req.Webroot != "" {
			args = append(args, "-w", req.Webroot)
		}
	case "dns":
		if req.DNSProvider != "" {
			args = append(args, "--dns", req.DNSProvider)
		}
	}

	output, err := exec.Command("acme.sh", args...).CombinedOutput()
	outStr := string(output)
	result := &IssueResult{Output: outStr}

	if err != nil {
		result.Error = err.Error()
		return result, err
	}

	result.Success = true
	result.CertPath = fmt.Sprintf("%s.cer", req.Domain)
	result.KeyPath = fmt.Sprintf("%s.key", req.Domain)
	result.FullchainPath = fmt.Sprintf("fullchain.cer")
	return result, nil
}

// Renew 续期证书
func Renew(domain string) (string, error) {
	output, err := exec.Command("acme.sh", "--renew", "-d", domain).CombinedOutput()
	return string(output), err
}

// Revoke 吊销证书
func Revoke(domain string) (string, error) {
	output, err := exec.Command("acme.sh", "--revoke", "-d", domain).CombinedOutput()
	return string(output), err
}

// Detect 检测 acme.sh 是否安装
func Detect() (installed bool, path string, version string) {
	p, err := exec.LookPath("acme.sh")
	if err != nil {
		return false, "", ""
	}
	output, err := exec.Command(p, "--version").CombinedOutput()
	if err == nil {
		version = strings.TrimSpace(string(output))
	}
	return true, p, version
}

// CertScheduler 证书到期扫描与自动续期
type CertScheduler struct {
	RenewBefore time.Duration
	RenewFunc   func(domain string) error
}

// NewCertScheduler 创建证书调度器
func NewCertScheduler(renewBeforeDays int, renewFunc func(domain string) error) *CertScheduler {
	return &CertScheduler{
		RenewBefore: time.Duration(renewBeforeDays) * 24 * time.Hour,
		RenewFunc:   renewFunc,
	}
}

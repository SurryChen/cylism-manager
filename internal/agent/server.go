package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	pb "github.com/cylism/cylism-manager/api/proto/agent"
)

// Server 实现 AgentService gRPC 服务
type Server struct {
	pb.UnimplementedAgentServiceServer
	Version string
}

// NewServer 创建 Agent 服务实例
func NewServer(version string) *Server {
	return &Server{Version: version}
}

// Ping 心跳
func (s *Server) Ping(ctx context.Context, req *pb.PingRequest) (*pb.PingResponse, error) {
	return &pb.PingResponse{Ok: true, Version: s.Version}, nil
}

// -- NGINX --

func (s *Server) NginxTest(ctx context.Context, req *pb.NginxTestRequest) (*pb.NginxTestResponse, error) {
	output, err := exec.Command("nginx", "-t").CombinedOutput()
	outStr := string(output)
	if ee, ok := err.(*exec.ExitError); ok {
		return &pb.NginxTestResponse{Ok: false, Output: outStr, Error: ee.Error()}, nil
	}
	if err != nil {
		return &pb.NginxTestResponse{Ok: false, Error: err.Error()}, nil
	}
	return &pb.NginxTestResponse{Ok: true, Output: outStr}, nil
}

func (s *Server) NginxReload(ctx context.Context, req *pb.NginxReloadRequest) (*pb.NginxReloadResponse, error) {
	output, err := exec.Command("nginx", "-s", "reload").CombinedOutput()
	outStr := string(output)
	if err != nil {
		return &pb.NginxReloadResponse{Ok: false, Output: outStr, Error: err.Error()}, nil
	}
	return &pb.NginxReloadResponse{Ok: true, Output: outStr}, nil
}

func (s *Server) NginxGetConfig(ctx context.Context, req *pb.NginxGetConfigRequest) (*pb.NginxGetConfigResponse, error) {
	output, err := exec.Command("nginx", "-T").CombinedOutput()
	if err != nil {
		return &pb.NginxGetConfigResponse{Error: err.Error()}, nil
	}
	return &pb.NginxGetConfigResponse{Config: string(output)}, nil
}

// -- Config Files --

func (s *Server) WriteConfigFile(ctx context.Context, req *pb.WriteConfigFileRequest) (*pb.WriteConfigFileResponse, error) {
	// 确保目录存在
	if err := os.MkdirAll(getDir(req.Path), 0755); err != nil {
		return &pb.WriteConfigFileResponse{Ok: false, Error: err.Error()}, nil
	}
	if err := os.WriteFile(req.Path, []byte(req.Content), 0644); err != nil {
		return &pb.WriteConfigFileResponse{Ok: false, Error: err.Error()}, nil
	}
	return &pb.WriteConfigFileResponse{Ok: true}, nil
}

func (s *Server) DeleteConfigFile(ctx context.Context, req *pb.DeleteConfigFileRequest) (*pb.DeleteConfigFileResponse, error) {
	if err := os.Remove(req.Path); err != nil && !os.IsNotExist(err) {
		return &pb.DeleteConfigFileResponse{Ok: false, Error: err.Error()}, nil
	}
	return &pb.DeleteConfigFileResponse{Ok: true}, nil
}

func (s *Server) FileRead(ctx context.Context, req *pb.FileReadRequest) (*pb.FileReadResponse, error) {
	data, err := os.ReadFile(req.Path)
	if err != nil {
		return &pb.FileReadResponse{Error: err.Error()}, nil
	}
	return &pb.FileReadResponse{Content: data}, nil
}

func (s *Server) FileStat(ctx context.Context, req *pb.FileStatRequest) (*pb.FileStatResponse, error) {
	info, err := os.Stat(req.Path)
	if os.IsNotExist(err) {
		return &pb.FileStatResponse{Exists: false}, nil
	}
	if err != nil {
		return &pb.FileStatResponse{Error: err.Error()}, nil
	}
	return &pb.FileStatResponse{
		Exists: true,
		Size:   info.Size(),
		Mode:   int64(info.Mode()),
	}, nil
}

// -- acme.sh --

func (s *Server) AcmeIssue(ctx context.Context, req *pb.AcmeIssueRequest) (*pb.AcmeIssueResponse, error) {
	args := []string{"--issue", "-d", req.Domain, "--force"}
	switch req.Challenge {
	case "http":
		args = append(args, "-w", req.Webroot)
	case "dns":
		if req.DnsProvider != "" {
			args = append(args, "--dns", req.DnsProvider)
		}
	}
	output, err := exec.Command("acme.sh", args...).CombinedOutput()
	outStr := string(output)

	if err != nil {
		return &pb.AcmeIssueResponse{Ok: false, Output: outStr, Error: err.Error()}, nil
	}

	// 从输出中提取证书路径（通常 acme.sh 会输出路径信息）
	// 标准路径模式: ~/.acme.sh/<domain>/
	home, _ := os.UserHomeDir()
	basePath := fmt.Sprintf("%s/.acme.sh/%s", home, req.Domain)

	return &pb.AcmeIssueResponse{
		Ok:            true,
		CertPath:      basePath + "/" + req.Domain + ".cer",
		KeyPath:       basePath + "/" + req.Domain + ".key",
		FullchainPath: basePath + "/fullchain.cer",
		Output:        outStr,
	}, nil
}

func (s *Server) AcmeRenew(ctx context.Context, req *pb.AcmeRenewRequest) (*pb.AcmeRenewResponse, error) {
	output, err := exec.Command("acme.sh", "--renew", "-d", req.Domain).CombinedOutput()
	outStr := string(output)
	if err != nil {
		return &pb.AcmeRenewResponse{Ok: false, Output: outStr, Error: err.Error()}, nil
	}
	return &pb.AcmeRenewResponse{Ok: true, Output: outStr}, nil
}

func (s *Server) AcmeRevoke(ctx context.Context, req *pb.AcmeRevokeRequest) (*pb.AcmeRevokeResponse, error) {
	output, err := exec.Command("acme.sh", "--revoke", "-d", req.Domain).CombinedOutput()
	outStr := string(output)
	if err != nil {
		return &pb.AcmeRevokeResponse{Ok: false, Output: outStr, Error: err.Error()}, nil
	}
	return &pb.AcmeRevokeResponse{Ok: true, Output: outStr}, nil
}

func (s *Server) AcmeDetect(ctx context.Context, req *pb.AcmeDetectRequest) (*pb.AcmeDetectResponse, error) {
	path, err := exec.LookPath("acme.sh")
	if err != nil {
		return &pb.AcmeDetectResponse{Installed: false}, nil
	}
	output, err := exec.Command(path, "--version").CombinedOutput()
	version := "unknown"
	if err == nil {
		version = strings.TrimSpace(string(output))
	}
	return &pb.AcmeDetectResponse{Installed: true, Path: path, Version: version}, nil
}

// -- SystemInfo --

func (s *Server) SystemInfo(ctx context.Context, req *pb.SystemInfoRequest) (*pb.SystemInfoResponse, error) {
	hostname, _ := os.Hostname()
	return &pb.SystemInfoResponse{
		Os:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		Hostname: hostname,
	}, nil
}

// getDir 返回文件路径的目录部分
func getDir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}

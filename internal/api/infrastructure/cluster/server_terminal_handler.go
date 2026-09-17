package cluster

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	crypto "github.com/cylism/cylism-manager/internal/security"
	auditservice "github.com/cylism/cylism-manager/internal/service/audit"
	"github.com/cylism/cylism-manager/internal/transport"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/ssh"
)

// ServerTerminalHandler owns the SSH terminal WebSocket transport for a
// registered server. Lifecycle and authorization remain outside this adapter.
type ServerTerminalHandler struct {
	store  repository.ServerRepository
	encKey []byte
	audit  auditservice.Repository
}

func NewServerTerminalHandler(st repository.ServerRepository, encKey []byte) *ServerTerminalHandler {
	return &ServerTerminalHandler{store: st, encKey: encKey}
}

func (h *ServerTerminalHandler) WithAudit(logs auditservice.Repository) *ServerTerminalHandler {
	h.audit = logs
	return h
}

func (h *ServerTerminalHandler) Terminal(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		return
	}
	server, err := h.store.GetServer(uint(id))
	if err != nil {
		return
	}
	conn, err := transport.WSUpgrade(c.Writer, c.Request)
	if err != nil {
		return
	}
	defer conn.Close()

	port := server.SSHPort
	if port == 0 {
		port = 22
	}
	sshConfig := &ssh.ClientConfig{User: server.SSHUser, HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 10 * time.Second}
	if server.SSHAuthType == "key" && server.SSHKey != "" {
		decKey, decryptErr := crypto.Decrypt(h.encKey, server.SSHKey)
		if decryptErr != nil {
			log.Printf("[terminal] key decrypt failed for server=%d: %v", server.ID, decryptErr)
			return
		}
		signer, parseErr := ssh.ParsePrivateKey([]byte(decKey))
		if parseErr != nil {
			log.Printf("[terminal] key parse failed for server=%d: %v", server.ID, parseErr)
			return
		}
		sshConfig.Auth = []ssh.AuthMethod{ssh.PublicKeys(signer)}
	} else if server.SSHAuthType == "password" && server.SSHPassword != "" {
		password, decryptErr := crypto.Decrypt(h.encKey, server.SSHPassword)
		if decryptErr != nil {
			log.Printf("[terminal] password decrypt failed for server=%d: %v", server.ID, decryptErr)
			return
		}
		sshConfig.Auth = []ssh.AuthMethod{ssh.Password(password)}
	} else {
		log.Printf("[terminal] no auth method for server=%d", server.ID)
		return
	}

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", server.Host, port), sshConfig)
	if err != nil {
		log.Printf("[terminal] dial failed for server=%d: %v", server.ID, err)
		return
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return
	}
	defer session.Close()
	if err := session.RequestPty("xterm-256color", 40, 80, ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 14400, ssh.TTY_OP_OSPEED: 14400}); err != nil {
		return
	}
	sessionIn, err := session.StdinPipe()
	if err != nil {
		return
	}
	sessionOut, err := session.StdoutPipe()
	if err != nil {
		return
	}
	if err := session.Shell(); err != nil {
		return
	}
	h.recordTerminalSession(c, server)

	go func() {
		for {
			data, readErr := conn.ReadFrame()
			if readErr != nil {
				_ = session.Close()
				return
			}
			if len(data) == 0 {
				continue
			}
			var resize struct {
				Type string `json:"type"`
				Cols int    `json:"cols"`
				Rows int    `json:"rows"`
			}
			if json.Unmarshal(data, &resize) == nil && resize.Type == "resize" && resize.Cols > 0 && resize.Rows > 0 {
				_ = session.WindowChange(resize.Rows, resize.Cols)
				continue
			}
			if _, writeErr := sessionIn.Write(data); writeErr != nil {
				_ = session.Close()
				return
			}
		}
	}()

	buffer := make([]byte, 4096)
	for {
		n, readErr := sessionOut.Read(buffer)
		if readErr != nil {
			break
		}
		if n > 0 {
			if writeErr := conn.WriteFrame(buffer[:n]); writeErr != nil {
				break
			}
		}
	}
}

func (h *ServerTerminalHandler) recordTerminalSession(c *gin.Context, server *model.Server) {
	if h == nil || h.audit == nil || server == nil {
		return
	}
	_ = auditservice.NewService(h.audit).Record(auditservice.AuditEventInput{
		Action: "server.terminal.start", ResourceType: "server", ResourceID: server.ID, TargetName: server.Name,
		Actor: apiShared.ActorFromContext(c), Source: model.AuditSourceAPI, Outcome: model.AuditOutcomeSucceeded,
		Summary: "启动服务器 Terminal 会话", RequestID: apiShared.RequestID(c), Metadata: map[string]any{"host": server.Host},
	})
}

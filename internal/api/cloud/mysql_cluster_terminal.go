package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/transport"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

const mysqlTerminalReadyMarker = "\x1eCYLISM_MYSQL_READY\x1e"

// The password is read after echo has been disabled. It never appears in the
// Kubernetes exec command, URL, or terminal output.
const mysqlTerminalCommand = "stty -echo || exit 1; printf '\\036CYLISM_MYSQL_READY\\036'; IFS= read -r MYSQL_PWD || exit 1; stty echo; export MYSQL_PWD; exec mysql --protocol=tcp --host=127.0.0.1 --user=root"

func validateMySQLTerminalPod(item *model.MySQLCluster, pod *corev1.Pod) error {
	if item.Status == "draft" || item.Status == "paused" {
		return fmt.Errorf("MySQL 集群尚未运行")
	}
	if pod.Namespace != item.Namespace || pod.Labels["mysql.oracle.com/cluster"] != item.ResourceName || pod.Labels["component"] != "mysqld" {
		return fmt.Errorf("目标 Pod 不属于当前 MySQL 集群")
	}
	if pod.DeletionTimestamp != nil || pod.Status.Phase != corev1.PodRunning {
		return fmt.Errorf("仅运行中的 MySQL 实例可打开数据库终端")
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			for _, container := range pod.Spec.Containers {
				if container.Name == "mysql" {
					return nil
				}
			}
			return fmt.Errorf("MySQL 容器不存在")
		}
	}
	return fmt.Errorf("MySQL 实例尚未就绪")
}

func (h *DatabaseHandler) MySQLClusterTerminal(c *gin.Context) {
	item, ok := h.getCluster(c)
	if !ok {
		return
	}
	if h.client == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	client, err := h.client(c.Request.Context(), item.ClusterID)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if client == nil || client.Clientset == nil {
		apiShared.K8sUnavailable(c)
		return
	}
	pod, err := client.Clientset.CoreV1().Pods(item.Namespace).Get(c.Request.Context(), c.Param("podName"), metav1.GetOptions{})
	if err != nil {
		apiShared.NotFound(c, "MySQL 实例不存在")
		return
	}
	if err := validateMySQLTerminalPod(item, pod); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	secret, err := client.Clientset.CoreV1().Secrets(item.Namespace).Get(c.Request.Context(), item.ResourceName+"-root", metav1.GetOptions{})
	if err != nil || len(secret.Data["rootPassword"]) == 0 {
		apiShared.ValidationError(c, "读取数据库管理凭据失败")
		return
	}
	password := secret.Data["rootPassword"]
	if bytes.Contains(password, []byte("\r")) || bytes.Contains(password, []byte("\n")) {
		apiShared.ValidationError(c, "数据库管理凭据格式无效")
		return
	}
	if client.Config == nil || client.Clientset.CoreV1().RESTClient() == nil {
		apiShared.K8sUnavailable(c)
		return
	}

	conn, err := transport.WSUpgrade(c.Writer, c.Request)
	if err != nil {
		return
	}
	defer conn.Close()
	request := client.Clientset.CoreV1().RESTClient().Post().Resource("pods").Namespace(item.Namespace).Name(pod.Name).SubResource("exec").VersionedParams(&corev1.PodExecOptions{
		Container: "mysql", Command: []string{"/bin/sh", "-c", mysqlTerminalCommand}, Stdin: true, Stdout: true, Stderr: true, TTY: true,
	}, scheme.ParameterCodec)
	streamConfig := rest.CopyConfig(client.Config)
	streamConfig.Timeout = 0
	executor, err := remotecommand.NewSPDYExecutor(streamConfig, http.MethodPost, request.URL())
	if err != nil {
		writeMySQLTerminalError(conn, err)
		return
	}
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	inputReader, inputWriter := io.Pipe()
	defer inputReader.Close()
	defer inputWriter.Close()
	output := &mysqlTerminalOutput{conn: conn, ready: make(chan struct{})}
	sizeQueue := newMySQLTerminalSizeQueue()
	done := make(chan error, 1)
	go func() {
		done <- executor.StreamWithContext(ctx, remotecommand.StreamOptions{
			Stdin: inputReader, Stdout: output, Stderr: output, Tty: true, TerminalSizeQueue: sizeQueue,
		})
		_ = inputReader.Close()
	}()

	select {
	case <-output.ready:
	case err := <-done:
		writeMySQLTerminalError(conn, err)
		return
	case <-time.After(15 * time.Second):
		writeMySQLTerminalError(conn, fmt.Errorf("等待 MySQL 客户端启动超时"))
		return
	}
	if _, err := inputWriter.Write(append(append([]byte(nil), password...), '\n')); err != nil {
		writeMySQLTerminalError(conn, err)
		return
	}
	go readMySQLTerminalInput(ctx, cancel, conn, inputWriter, sizeQueue)
	if err := <-done; err != nil && ctx.Err() == nil {
		writeMySQLTerminalError(conn, err)
	}
}

type mysqlTerminalOutput struct {
	conn    *transport.WSConn
	ready   chan struct{}
	pending []byte
	started bool
}

func (w *mysqlTerminalOutput) Write(data []byte) (int, error) {
	if w.started {
		return len(data), w.conn.WriteFrame(data)
	}
	w.pending = append(w.pending, data...)
	if index := bytes.Index(w.pending, []byte(mysqlTerminalReadyMarker)); index >= 0 {
		remaining := append([]byte(nil), w.pending[index+len(mysqlTerminalReadyMarker):]...)
		w.pending = nil
		w.started = true
		close(w.ready)
		if len(remaining) > 0 {
			return len(data), w.conn.WriteFrame(remaining)
		}
	} else if len(w.pending) > 4096 {
		return 0, fmt.Errorf("MySQL 客户端启动失败")
	}
	return len(data), nil
}

type mysqlTerminalSizeQueue struct {
	sizes chan remotecommand.TerminalSize
}

func newMySQLTerminalSizeQueue() *mysqlTerminalSizeQueue {
	queue := &mysqlTerminalSizeQueue{sizes: make(chan remotecommand.TerminalSize, 1)}
	queue.Resize(80, 24)
	return queue
}

func (q *mysqlTerminalSizeQueue) Next() *remotecommand.TerminalSize {
	size, ok := <-q.sizes
	if !ok {
		return nil
	}
	return &size
}

func (q *mysqlTerminalSizeQueue) Resize(cols, rows uint16) {
	if cols == 0 || rows == 0 {
		return
	}
	select {
	case <-q.sizes:
	default:
	}
	select {
	case q.sizes <- remotecommand.TerminalSize{Width: cols, Height: rows}:
	default:
	}
}

func readMySQLTerminalInput(ctx context.Context, cancel context.CancelFunc, conn *transport.WSConn, input io.WriteCloser, sizes *mysqlTerminalSizeQueue) {
	defer cancel()
	defer input.Close()
	for {
		data, err := conn.ReadFrame()
		if err != nil {
			return
		}
		var resize struct {
			Type string `json:"type"`
			Cols uint16 `json:"cols"`
			Rows uint16 `json:"rows"`
		}
		if json.Unmarshal(data, &resize) == nil && resize.Type == "resize" {
			sizes.Resize(resize.Cols, resize.Rows)
			continue
		}
		select {
		case <-ctx.Done():
			return
		default:
			if _, err := input.Write(data); err != nil {
				return
			}
		}
	}
}

func writeMySQLTerminalError(conn *transport.WSConn, err error) {
	if err == nil {
		err = fmt.Errorf("MySQL 客户端已退出")
	}
	_ = conn.WriteJSON(gin.H{"type": "error", "detail": fmt.Sprintf("数据库终端连接失败: %v", err)})
}

var _ remotecommand.TerminalSizeQueue = (*mysqlTerminalSizeQueue)(nil)
var _ io.Writer = (*mysqlTerminalOutput)(nil)

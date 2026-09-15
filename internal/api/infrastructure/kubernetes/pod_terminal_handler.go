package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	auditservice "github.com/cylism/cylism-manager/internal/service/audit"
	"github.com/cylism/cylism-manager/internal/transport"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"

	"github.com/gin-gonic/gin"
)

type podTerminalResizeMessage struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

type podTerminalSizeQueue struct {
	sizes chan remotecommand.TerminalSize
}

func newPodTerminalSizeQueue() *podTerminalSizeQueue {
	queue := &podTerminalSizeQueue{sizes: make(chan remotecommand.TerminalSize, 1)}
	queue.Resize(80, 24)
	return queue
}

func (q *podTerminalSizeQueue) Next() *remotecommand.TerminalSize {
	size, ok := <-q.sizes
	if !ok {
		return nil
	}
	return &size
}

func (q *podTerminalSizeQueue) Resize(cols, rows uint16) {
	if cols == 0 || rows == 0 {
		return
	}
	size := remotecommand.TerminalSize{Width: cols, Height: rows}
	select {
	case <-q.sizes:
	default:
	}
	select {
	case q.sizes <- size:
	default:
	}
}

type podTerminalOutput struct {
	conn *transport.WSConn
}

func (w podTerminalOutput) Write(data []byte) (int, error) {
	if err := w.conn.WriteFrame(data); err != nil {
		return 0, err
	}
	return len(data), nil
}

// PodTerminal opens an interactive shell in one running Pod container.
func (h *K8sHandler) PodTerminal(c *gin.Context) {
	if h.k8s == nil || !h.k8s.KubernetesAvailable() || h.k8s.KubeConfig() == nil {
		apiShared.K8sUnavailable(c)
		return
	}

	namespace, name := c.Param("namespace"), c.Param("name")
	pod, err := h.k8s.CoreV1().Pods(namespace).Get(c.Request.Context(), name, metav1.GetOptions{})
	if err != nil {
		apiShared.NotFound(c, fmt.Sprintf("获取 Pod 失败: %v", err))
		return
	}
	if pod.Status.Phase != corev1.PodRunning {
		apiShared.BadRequest(c, "仅运行中的 Pod 可打开终端")
		return
	}

	container, err := selectPodTerminalContainer(pod, c.Query("container"))
	if err != nil {
		apiShared.BadRequest(c, err.Error())
		return
	}

	conn, err := transport.WSUpgrade(c.Writer, c.Request)
	if err != nil {
		return
	}
	defer conn.Close()
	h.recordPodTerminalSession(c, namespace, name, container)

	request := h.k8s.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(namespace).
		Name(name).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   []string{"/bin/sh"},
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       true,
		}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(h.k8s.KubeConfig(), http.MethodPost, request.URL())
	if err != nil {
		writePodTerminalError(conn, err)
		return
	}

	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	inputReader, inputWriter := io.Pipe()
	defer inputReader.Close()
	defer inputWriter.Close()
	sizeQueue := newPodTerminalSizeQueue()

	go readPodTerminalInput(ctx, cancel, conn, inputWriter, sizeQueue)
	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:             inputReader,
		Stdout:            podTerminalOutput{conn: conn},
		Stderr:            podTerminalOutput{conn: conn},
		Tty:               true,
		TerminalSizeQueue: sizeQueue,
	})
	if err != nil && ctx.Err() == nil {
		writePodTerminalError(conn, err)
	}
}

func selectPodTerminalContainer(pod *corev1.Pod, requested string) (string, error) {
	if requested == "" && len(pod.Spec.Containers) == 1 {
		return pod.Spec.Containers[0].Name, nil
	}
	for _, container := range pod.Spec.Containers {
		if container.Name == requested {
			return container.Name, nil
		}
	}
	if requested == "" {
		return "", fmt.Errorf("该 Pod 有多个容器，请选择要进入的容器")
	}
	return "", fmt.Errorf("容器 %q 不属于该 Pod", requested)
}

func readPodTerminalInput(ctx context.Context, cancel context.CancelFunc, conn *transport.WSConn, input io.WriteCloser, sizeQueue *podTerminalSizeQueue) {
	defer cancel()
	defer input.Close()
	for {
		data, err := conn.ReadFrame()
		if err != nil {
			return
		}
		var resize podTerminalResizeMessage
		if json.Unmarshal(data, &resize) == nil && resize.Type == "resize" {
			sizeQueue.Resize(resize.Cols, resize.Rows)
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

func writePodTerminalError(conn *transport.WSConn, err error) {
	_ = conn.WriteJSON(gin.H{"type": "error", "detail": fmt.Sprintf("容器终端连接失败: %v", err)})
}

func (h *K8sHandler) recordPodTerminalSession(c *gin.Context, namespace, name, container string) {
	if h.audit == nil {
		return
	}
	_ = auditservice.NewService(h.audit).Record(auditservice.AuditEventInput{
		Action: "workload.pod.terminal.start", ResourceType: "pod", TargetName: namespace + "/" + name,
		Actor: apiShared.ActorFromContext(c), Source: model.AuditSourceAPI, Outcome: model.AuditOutcomeSucceeded,
		Summary: "启动 Pod Terminal 会话", RequestID: apiShared.RequestID(c), Metadata: map[string]any{"namespace": namespace, "pod": name, "container": container},
	})
}

var _ remotecommand.TerminalSizeQueue = (*podTerminalSizeQueue)(nil)
var _ io.Writer = podTerminalOutput{}

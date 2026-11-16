package cluster

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type fakeRemovalClient struct {
	node     *k8s.RemovalNode
	plan     *k8s.DrainPlan
	events   *[]string
	drainErr error
	pending  bool
}

func (f *fakeRemovalClient) GetRemovalNodeContext(context.Context, string) (*k8s.RemovalNode, error) {
	if f.node == nil {
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "nodes"}, "worker")
	}
	copy := *f.node
	return &copy, nil
}
func (f *fakeRemovalClient) DrainPlanContext(context.Context, string) (*k8s.DrainPlan, error) {
	return f.plan, nil
}
func (f *fakeRemovalClient) DrainRemovalNodeContext(_ context.Context, _ string, _ string, options k8s.DrainOptions) (*k8s.DrainResult, error) {
	*f.events = append(*f.events, "drain")
	if len(f.plan.Blocked) > 0 || (len(f.plan.RequiresEmptyDirConfirmation) > 0 && !options.DeleteEmptyDirData) {
		return nil, errors.New("blocked")
	}
	if f.drainErr != nil {
		return nil, f.drainErr
	}
	if !f.pending {
		f.plan = &k8s.DrainPlan{}
	}
	return &k8s.DrainResult{}, nil
}
func (f *fakeRemovalClient) DeleteRemovalNodeContext(context.Context, string, string) error {
	*f.events = append(*f.events, "delete")
	f.node = nil
	return nil
}

type fakeRemovalHost struct {
	client    *fakeRemovalClient
	stopErr   error
	checkErr  error
	keepReady bool
}

func (f *fakeRemovalHost) Check(context.Context, *model.Server, *model.ClusterDeployment) error {
	return f.checkErr
}
func (f *fakeRemovalHost) Stop(context.Context, *model.Server, *model.ClusterDeployment) error {
	*f.client.events = append(*f.client.events, "stop")
	if f.stopErr != nil {
		return f.stopErr
	}
	if f.client.node != nil && !f.keepReady {
		f.client.node.Ready = false
	}
	return nil
}

func removalHandlerFixture(t *testing.T) (*DeploymentHandler, *store.Store, *model.ClusterDeployment, *fakeRemovalClient, *fakeRemovalHost) {
	t.Helper()
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	controlID := uint(1)
	cluster := &model.Cluster{ID: 1, Name: "cluster", Distribution: "k3s", Status: "ready", ControlServerID: &controlID}
	control := &model.Server{ID: 1, Name: "control", Host: "192.0.2.1", ClusterID: &cluster.ID, ClusterRole: "control-plane", K8sNodeName: "control", SSHPassword: "secret"}
	worker := &model.Server{ID: 2, Name: "worker", Host: "192.0.2.2", ClusterID: &cluster.ID, ClusterRole: "worker", K8sNodeName: "worker", SSHPassword: "secret"}
	for _, item := range []any{cluster, control, worker} {
		if err = st.DB().Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}
	events := []string{}
	client := &fakeRemovalClient{node: &k8s.RemovalNode{Name: "worker", UID: "uid-1", MachineID: "machine-1", Ready: true, PodCIDRs: []string{"10.42.1.0/24"}}, plan: &k8s.DrainPlan{Evictable: []k8s.DrainPod{{Name: "web"}}}, events: &events}
	host := &fakeRemovalHost{client: client}
	h := NewDeploymentHandler(st, nil).WithNodeRemovalClientFactory(func(context.Context, uint) (NodeRemovalClient, error) { return client, nil })
	h.removalHost = host
	h.removalPollInterval = time.Millisecond
	identity := &model.ClusterDeployment{NodeName: "worker", NodeUID: "uid-1", NodeMachineID: "machine-1", NodePodCIDRs: `["10.42.1.0/24"]`}
	return h, st, identity, client, host
}

func TestRemovalTaskOrdersDrainStopDeleteAndUnbind(t *testing.T) {
	h, st, identity, client, _ := removalHandlerFixture(t)
	task, err := st.StartClusterNodeRemoval(1, 2, identity)
	if err != nil {
		t.Fatal(err)
	}
	h.runNodeRemoval(task, k8s.DrainOptions{})
	if !reflect.DeepEqual(*client.events, []string{"drain", "stop", "delete", "stop"}) {
		t.Fatalf("wrong order: %v", *client.events)
	}
	got, _ := st.GetClusterDeployment(1, task.ID)
	server, _ := st.GetServer(2)
	if got.Status != "success" || !got.NodeStopped || server.ClusterID != nil {
		t.Fatalf("not completed: %#v %#v", got, server)
	}
}

func TestRemovalRecoversMissingNodeAfterStoppedCheckpoint(t *testing.T) {
	h, st, identity, client, _ := removalHandlerFixture(t)
	previous, err := st.StartClusterNodeRemoval(1, 2, identity)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.MarkClusterNodeRemovalStopped(previous.ID); err != nil {
		t.Fatal(err)
	}
	if err = st.FinishClusterNodeRemoval(previous, false); err != nil {
		t.Fatal(err)
	}
	client.node = nil
	server, err := st.GetServer(2)
	if err != nil {
		t.Fatal(err)
	}
	_, checkpoint, err := h.prepareRemoval(context.Background(), 1, server)
	if err != nil {
		t.Fatal(err)
	}
	task, err := st.StartClusterNodeRemoval(1, 2, checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	h.runNodeRemoval(task, k8s.DrainOptions{})
	got, err := st.GetClusterDeployment(1, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	server, err = st.GetServer(2)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "success" || server.ClusterID != nil || !reflect.DeepEqual(*client.events, []string{"stop", "stop"}) {
		t.Fatalf("recovery failed: %#v %#v %v", got, server, *client.events)
	}
}

func TestRemovalWaitPassesDeadlineToCheck(t *testing.T) {
	h := &DeploymentHandler{}
	err := h.waitRemoval(context.Background(), 10*time.Millisecond, func(ctx context.Context) (bool, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("check received no deadline")
		}
		<-ctx.Done()
		return false, ctx.Err()
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline cancellation, got %v", err)
	}
}

func TestRemovalFailuresPreserveBindingAndDoNotDelete(t *testing.T) {
	for _, failure := range []string{"unmanaged", "emptydir", "host", "identity", "pdb", "still-ready"} {
		t.Run(failure, func(t *testing.T) {
			h, st, identity, client, host := removalHandlerFixture(t)
			task, err := st.StartClusterNodeRemoval(1, 2, identity)
			if err != nil {
				t.Fatal(err)
			}
			switch failure {
			case "unmanaged":
				client.plan.Blocked = []k8s.DrainPod{{Name: "debug"}}
			case "emptydir":
				client.plan.RequiresEmptyDirConfirmation = []k8s.DrainPod{{Name: "cache"}}
			case "host":
				host.stopErr = errors.New("SSH failed")
			case "identity":
				client.node.UID = "replacement"
			case "pdb":
				client.pending = true
			case "still-ready":
				host.keepReady = true
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
			defer cancel()
			err = h.removeWorker(ctx, task, k8s.DrainOptions{}, func(string) {})
			if err == nil {
				t.Fatal("expected failure")
			}
			if err = st.FinishClusterNodeRemoval(task, false); err != nil {
				t.Fatal(err)
			}
			for _, event := range *client.events {
				if event == "delete" || (event == "stop" && failure != "host" && failure != "still-ready") {
					t.Fatalf("unsafe event: %v", *client.events)
				}
			}
			server, _ := st.GetServer(2)
			if server.ClusterID == nil {
				t.Fatal("failure unbound server")
			}
		})
	}
}

func TestRemovalPreflightScopesAndConfirmation(t *testing.T) {
	h, st, identity, client, _ := removalHandlerFixture(t)
	router := gin.New()
	router.GET("/clusters/:id/nodes/:serverID/removal-plan", h.NodeRemovalPlan)
	router.POST("/clusters/:id/nodes/:serverID/remove", h.RemoveNode)
	for _, path := range []string{"/clusters/1/nodes/1/removal-plan", "/clusters/2/nodes/2/removal-plan", "/clusters/no/nodes/2/removal-plan"} {
		if response := serve(router, newJSONRequest(http.MethodGet, path, nil)); response.Code == 200 {
			t.Fatalf("unsafe scope accepted: %s", path)
		}
	}
	for _, body := range []any{map[string]any{"confirm_node_name": "wrong", "node_uid": "uid-1"}, map[string]any{"confirm_node_name": "worker", "node_uid": "stale"}} {
		if response := serve(router, newJSONRequest(http.MethodPost, "/clusters/1/nodes/2/remove", body)); response.Code == 200 {
			t.Fatal("bad confirmation accepted")
		}
	}
	client.plan.RequiresEmptyDirConfirmation = []k8s.DrainPod{{Name: "cache"}}
	if response := serve(router, newJSONRequest(http.MethodPost, "/clusters/1/nodes/2/remove", map[string]any{"confirm_node_name": "worker", "node_uid": "uid-1"})); response.Code != 400 {
		t.Fatalf("emptyDir accepted: %d", response.Code)
	}
	client.node = nil
	server, _ := st.GetServer(2)
	if _, _, err := h.prepareRemoval(context.Background(), 1, server); err == nil {
		t.Fatal("missing node without checkpoint accepted")
	}
	task, err := st.StartClusterNodeRemoval(1, 2, identity)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.FinishClusterNodeRemoval(task, false); err != nil {
		t.Fatal(err)
	}
	if _, _, err = h.prepareRemoval(context.Background(), 1, server); err == nil {
		t.Fatal("missing node without stopped checkpoint accepted")
	}
	task, err = st.StartClusterNodeRemoval(1, 2, identity)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.MarkClusterNodeRemovalStopped(task.ID); err != nil {
		t.Fatal(err)
	}
	if err = st.FinishClusterNodeRemoval(task, false); err != nil {
		t.Fatal(err)
	}
	plan, _, err := h.prepareRemoval(context.Background(), 1, server)
	if err != nil || !plan.NodeMissing {
		t.Fatalf("checkpoint retry rejected: %#v %v", plan, err)
	}
	client.node = &k8s.RemovalNode{Name: "worker", UID: "replacement", MachineID: "machine-1"}
	if _, _, err = h.prepareRemoval(context.Background(), 1, server); err == nil {
		t.Fatal("replacement node accepted")
	}
}

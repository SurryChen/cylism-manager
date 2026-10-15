package database

import (
	"context"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestValidateMyCNFProtectsOperatorSettings(t *testing.T) {
	valid, err := ValidateMyCNF("[mysqld]\ninnodb_buffer_pool_size=128M\nperformance_schema_digests_size=1000")
	if err != nil || valid != "[mysqld]\ninnodb_buffer_pool_size=128M\nperformance_schema_digests_size=1000\n" {
		t.Fatalf("valid config = %q, %v", valid, err)
	}
	for _, input := range []string{"performance_schema=OFF", "[client]\nport=3306", "[mysqld]\ngroup_replication_start_on_boot=OFF", "[mysqld]\n!includedir /tmp", "[mysqld]\nmax_connections=20\nmax_connections=40"} {
		if _, err := ValidateMyCNF(input); err == nil {
			t.Errorf("accepted invalid config %q", input)
		}
	}
}

func TestMyCNFUpdateAndRestartPreserveCluster(t *testing.T) {
	ctx := context.Background()
	item := &model.MySQLCluster{ResourceName: "mysql-test", Namespace: "mysql", DatabaseNodes: []string{"node-a"}, DatabaseInstances: 1, StorageSize: "10Gi", Status: "ready"}
	object := ClusterObject(item)
	client := &k8s.Client{
		DynamicClient: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{innoDBClusterGVR: "InnoDBClusterList"}, object),
		Clientset:     k8sfake.NewSimpleClientset(&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: item.ResourceName, Namespace: item.Namespace}, Spec: appsv1.StatefulSetSpec{Replicas: int32ptr(1)}}),
	}
	config := "[mysqld]\ninnodb_buffer_pool_size=128M\n"
	if err := UpdateMyCNF(ctx, client, item, config); err != nil {
		t.Fatal(err)
	}
	item.ConfigRestartRequired = true
	actual, pending, err := MyCNFState(ctx, client, item)
	if err != nil || actual != config || !pending {
		t.Fatalf("config state = %q, pending=%v, err=%v", actual, pending, err)
	}
	if err := RestartMySQLForConfig(ctx, client, item, config); err != nil {
		t.Fatal(err)
	}
	item.ConfigRestartRequired = false
	_, pending, err = MyCNFState(ctx, client, item)
	if err != nil || pending {
		t.Fatalf("restart still pending=%v, err=%v", pending, err)
	}
	if err := UpdateMyCNF(ctx, client, item, ""); err != nil {
		t.Fatal(err)
	}
	item.ConfigRestartRequired = true
	_, pending, err = MyCNFState(ctx, client, item)
	if err != nil || !pending {
		t.Fatalf("clearing config must require restart: pending=%v, err=%v", pending, err)
	}
	if _, err := client.DynamicClient.Resource(innoDBClusterGVR).Namespace(item.Namespace).Get(ctx, item.ResourceName, metav1.GetOptions{}); err != nil {
		t.Fatalf("cluster resource was removed: %v", err)
	}
}

func int32ptr(value int32) *int32 { return &value }

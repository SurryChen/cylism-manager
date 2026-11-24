package store

import (
	"errors"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
)

func TestAccessCertificateUsageAndDeletion(t *testing.T) {
	st := setupTestDB(t)
	cluster := model.Cluster{Name: "证书集群", Distribution: "k3s", Status: "draft"}
	if err := st.CreateCluster(&cluster); err != nil {
		t.Fatal(err)
	}
	first := model.AccessCertificate{ClusterID: cluster.ID, Kind: "agent_server", Serial: "1001", Subject: "agent-1", CertificatePEM: "cert", Fingerprint: "fingerprint-1"}
	second := model.AccessCertificate{ClusterID: cluster.ID, Kind: "agent_server", Serial: "1002", Subject: "agent-2", CertificatePEM: "cert", Fingerprint: "fingerprint-2"}
	if err := st.CreateAccessCertificate(&first); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateAccessCertificate(&second); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkAccessCertificateInUse(first.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkAccessCertificateInUse(second.ID, true); err != nil {
		t.Fatal(err)
	}
	active, err := st.GetAccessCertificate(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if active.InUse {
		t.Fatal("older certificate remained in use")
	}
	if err := st.DeleteAccessCertificate(second.ID); err == nil {
		t.Fatal("expected in-use certificate deletion to fail")
	} else if !errors.Is(err, ErrAccessCertificateInUse) {
		t.Fatalf("unexpected deletion error: %v", err)
	}
	if err := st.DeleteAccessCertificate(first.ID); err != nil {
		t.Fatalf("delete unused certificate: %v", err)
	}
	if _, err := st.GetAccessCertificate(first.ID); err == nil {
		t.Fatal("unused certificate still exists")
	}
}

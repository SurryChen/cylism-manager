package maintenance

import (
	"strings"
	"testing"
)

func TestParseInspectionProducesBoundedStructuredData(t *testing.T) {
	output := `__CYLISM_FILESYSTEMS__
/ 42949672960 35567697920 7381975040 83%
/var/lib/containerd 21474836480 10737418240 10737418240 50%
__CYLISM_JOURNAL__
Archived and active journals take up 2.5G in the file system.
__CYLISM_DIR__:/var/log
2147483648 /var/log
1610612736 /var/log/journal
536870912 /var/log/pods
__CYLISM_DIR__:/var/lib/rancher/k3s
4294967296 /var/lib/rancher/k3s
3221225472 /var/lib/rancher/k3s/agent
`
	inspection, err := ParseInspection(output)
	if err != nil {
		t.Fatalf("parse inspection: %v", err)
	}
	if len(inspection.Filesystems) != 2 || inspection.Filesystems[0].MountPoint != "/" || inspection.Filesystems[0].UsedPercent != 83 {
		t.Fatalf("unexpected filesystems: %#v", inspection.Filesystems)
	}
	if inspection.JournalBytes != 2684354560 {
		t.Fatalf("unexpected journal bytes: %d", inspection.JournalBytes)
	}
	if len(inspection.Directories) != 2 || inspection.Directories[0].Path != "/var/log" || len(inspection.Directories[0].Entries) != 2 {
		t.Fatalf("unexpected directory data: %#v", inspection.Directories)
	}
}

func TestInspectionCommandIsFixedAndBounded(t *testing.T) {
	command := InspectionCommand()
	for _, path := range inspectionPaths {
		if !strings.Contains(command, path) {
			t.Fatalf("missing allowlisted path %q: %s", path, command)
		}
	}
	if strings.Contains(command, "$CYLISM_") || !strings.Contains(command, "timeout 12s") || !strings.Contains(command, "head -n 20") {
		t.Fatalf("inspection command must stay fixed and bounded: %s", command)
	}
}

func TestParseInspectionRejectsMissingFilesystemSection(t *testing.T) {
	if _, err := ParseInspection("__CYLISM_JOURNAL__\nnone\n"); err == nil {
		t.Fatal("expected malformed inspection to fail")
	}
}

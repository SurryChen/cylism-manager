package operations

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/security"
)

func TestDiscoverQueuesReadOnlyTaskWithoutSnapshotOrReplicationPassword(t *testing.T) {
	s, _ := testService(t)
	password, _ := security.Encrypt(s.key, "test-admin-password")
	source := model.DatabaseInstance{Name: "RDS", Mode: "external", Host: "source", Port: 3306, AdminUsername: "admin", EncryptedAdmin: password}
	target := model.DatabaseInstance{Name: "Replica", Mode: "managed", ClusterID: 1, Host: "target", Port: 3306, EncryptedRoot: password}
	mustCreate(t, s, &source)
	mustCreate(t, s, &target)
	task, err := s.Queue(context.Background(), Request{Kind: "replication_discover", Source: Reference(source.ID), Target: Reference(target.ID), ClusterID: 1, NodeName: "worker"})
	if err != nil {
		t.Fatal(err)
	}
	job := buildJob(task, Request{Image: DefaultImage})
	if job.Spec.Template.Spec.Volumes[1].EmptyDir == nil {
		t.Fatal("discovery requested snapshot storage")
	}
	s.execute = func(context.Context, *model.DatabaseOperation, Request) (bool, string, error) {
		return true, `{"channel":"existing","source_verified":true,"io_running":true,"sql_running":true,"lag_seconds":0,"read_only":true}`, nil
	}
	s.Reconcile(context.Background())
	var relation model.DatabaseReplication
	if err := s.store.DB().First(&relation).Error; err != nil {
		t.Fatal(err)
	}
	if relation.Channel != "existing" || relation.Source != task.Source || relation.LastCheckedAt == nil || relation.LagSeconds == nil || *relation.LagSeconds != 0 {
		t.Fatalf("bad observation: %+v", relation)
	}
	if readTask(t, s, task.ID).Status != "succeeded" {
		t.Fatal("discovery did not complete")
	}
}

func TestDiscoveryDoesNotRegisterUnverifiedOrConflictingRelations(t *testing.T) {
	s, _ := testService(t)
	task := model.DatabaseOperation{Kind: "replication_discover", Source: "instance:1", Target: "instance:2", ID: 1}
	if err := recordReplicationObservation(s.store.DB(), &task, `{"channel":"a"}`); err == nil {
		t.Fatal("adopted an unverified source")
	}
	mustCreate(t, s, &model.DatabaseReplication{Source: task.Source, Target: task.Target, Channel: "a", Status: "running"})
	result := `{"channel":"a","source_verified":true,"io_running":false,"sql_running":true,"lag_seconds":null,"read_only":true,"password":"never-persist"}`
	if err := recordReplicationObservation(s.store.DB(), &task, result); err != nil {
		t.Fatal(err)
	}
	if err := recordReplicationObservation(s.store.DB(), &task, result); err != nil {
		t.Fatal("not idempotent:", err)
	}
	var relations []model.DatabaseReplication
	s.store.DB().Find(&relations)
	if len(relations) != 1 || relations[0].Status != "error" || relations[0].LagSeconds != nil || strings.Contains(relations[0].Observation, "password") {
		t.Fatal("incorrect or unsafe observation")
	}
	task.Source = "instance:3"
	if err := recordReplicationObservation(s.store.DB(), &task, result); err == nil {
		t.Fatal("overwrote another source")
	}
	task.Source = "instance:1"
	if err := recordReplicationObservation(s.store.DB(), &task, `{"channel":"b","source_verified":true}`); err == nil {
		t.Fatal("overwrote another channel")
	}
}

func TestDiscoveryScriptOnlyReadsAndMatchesSourceUUID(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	// Execute the actual helper functions, excluding the script's global I/O
	// redirection and mounted-secret entry point. Reject every non-read SQL.
	program := `import ast,sys,json
tree=ast.parse(json.load(sys.stdin))
names={'Rejected','require','scalar','replica_channels','replication_observation','discover_replication'}
exec(compile(ast.Module(body=[n for n in tree.body if isinstance(n,(ast.FunctionDef,ast.ClassDef)) and n.name in names],type_ignores=[]),'<helpers>','exec'))
class Result:
 def __init__(self, rows): self.rows=rows
 def fetch_one(self): return self.rows[0]
 def fetch_all(self): return self.rows
 def get_column_names(self): return list(row)
row={'Source_UUID':'source-id','Channel_Name':"existing'channel",'Replica_IO_Running':'Yes','Replica_SQL_Running':'Yes','Seconds_Behind_Source':0,'Source_Password':'secret'}
class Session:
 def __init__(self, uuid): self.uuid=uuid; self.rows=[list(row.values())]
 def run_sql(self, sql):
  assert sql.startswith('SELECT ') or sql=='SHOW REPLICA STATUS',sql
  if sql=='SHOW REPLICA STATUS': return Result(self.rows)
  return Result([[self.uuid if 'server_uuid' in sql else 1]])
source,target=Session('source-id'),Session('target-id')
result=discover_replication(source,target)
assert result['source_verified'] and result['lag_seconds']==0 and 'Source_Password' not in result
source.uuid='wrong'
try: discover_replication(source,target); raise AssertionError('accepted wrong source')
except Rejected as error: assert error.code=='replication_source_mismatch'
target.rows*=2
try: discover_replication(source,target); raise AssertionError('accepted multiple channels')
except Rejected as error: assert error.code=='replication_multiple_channels'
target.rows=[]
try: discover_replication(source,target); raise AssertionError('accepted missing channel')
except Rejected as error: assert error.code=='replication_missing'
print('verified')
`
	raw, _ := json.Marshal(jobScript)
	cmd := exec.Command(python, "-c", program)
	cmd.Stdin = strings.NewReader(string(raw))
	if output, err := cmd.CombinedOutput(); err != nil || !strings.Contains(string(output), "verified") {
		t.Fatalf("helper contract failed: %v\n%s", err, output)
	}
}

#!/usr/bin/env python3
"""One-off background migration for cluster 2, 2026-11-06. Run on the source control plane as root.

Requires old platform recovery task 3 to be terminal before starting.
Never rerun this operation against its existing target PVC.
"""
import hashlib
import signal
import shutil
import copy
import datetime
import fcntl
import json
from pathlib import Path
import shlex
import subprocess
import sys
import time

component = sys.argv[1]
assert component in ('victoria-metrics', 'loki')
resume = False
assert len(sys.argv) == 2, 'Resume requires a fresh snapshot; do not restart this job'
name = 'cylism-' + component
kind = 'deployment' if component == 'victoria-metrics' else 'statefulset'
volume = 'storage' if component == 'victoria-metrics' else 'data'
target = 'ubuntudev'
operation = 'bg-20261106'
helper = name + '-transfer-' + operation
claim_name = name + '-data-' + operation
lock_name = name + '-migration-lock'
root = Path('/var/log/cylism-observability-' + operation)
root.mkdir(mode=0o700, exist_ok=True)
guard = (root / (component + '.process-lock')).open('w')
fcntl.flock(guard, fcntl.LOCK_EX | fcntl.LOCK_NB)
log = (root / (component + '.log')).open('a', buffering=1)

def note(msg):
    line = datetime.datetime.now().isoformat(timespec='seconds') + ' ' + msg
    print(line, flush=True)
    log.write(line + '\n')

def k(*args, data=None, timeout=90):
    p = subprocess.run(['kubectl', '-n', 'monitoring', *args], input=None if data is None else json.dumps(data), capture_output=True, text=True, timeout=timeout)
    if p.returncode:
        raise RuntimeError('kubectl ' + ' '.join(args[:4]) + ': ' + p.stderr.strip())
    return p.stdout

def get(kind, name):
    return json.loads(k('get', kind, name, '-o', 'json'))

def create(obj):
    return k('create', '-f', '-', data=obj)

def patch(spec):
    current = get(kind, name)
    if current['metadata']['uid'] != saved['metadata']['uid']:
        raise RuntimeError('Workload UID changed; refusing to patch')
    k('patch', kind, name, '--type=merge', '-p', json.dumps({'spec': spec}))

def workload_pods():
    return json.loads(k('get', 'pods', '-l', selector, '-o', 'json'))['items']

def wait_stopped():
    deadline = time.monotonic() + 300
    while time.monotonic() < deadline:
        if not workload_pods():
            return
        time.sleep(2)
    raise RuntimeError('Source pods did not stop')

def wait_running(node, pvc):
    deadline = time.monotonic() + 1200
    while time.monotonic() < deadline:
        pods = workload_pods()
        if len(pods) == 1:
            p = pods[0]
            ready = any(c['type'] == 'Ready' and c['status'] == 'True' for c in p.get('status', {}).get('conditions', []))
            attached = any(v.get('persistentVolumeClaim', {}).get('claimName') == pvc for v in p['spec']['volumes'])
            if ready and attached and p['spec']['nodeName'] == node and not p['metadata'].get('deletionTimestamp'):
                return p['metadata']['name']
        time.sleep(3)
    raise RuntimeError('Workload did not become ready')

def digest(args):
    out = subprocess.check_output(args, text=True, timeout=600).strip()
    value = out.split()[0]
    assert len(value) == 64, out
    return value

locked = False
stopped = resume
healthy = False
live = get(kind, name)
saved = json.loads((root / (component + '-workload.json')).read_text()) if resume else live
assert live['metadata']['uid'] == saved['metadata']['uid']
if resume:
    assert live['spec']['replicas'] == 0
source_spec = copy.deepcopy(saved['spec'])
source_node = source_spec['template']['spec']['nodeSelector']['kubernetes.io/hostname']
assert source_node == 'vm-0-16-ubuntu', source_node
assert source_spec['replicas'] == 1
selector = ','.join(key + '=' + val for key, val in source_spec['selector']['matchLabels'].items())
source_pvc = next(v['persistentVolumeClaim']['claimName'] for v in source_spec['template']['spec']['volumes'] if v['name'] == volume)
source_claim = get('pvc', source_pvc)
pv = get('pv', source_claim['spec']['volumeName'])
source_path = pv['spec'].get('local', pv['spec'].get('hostPath', {}))['path']
assert source_path.startswith('/var/lib/rancher/k3s/storage/')
node = get('node', target)
assert any(c['type'] == 'Ready' and c['status'] == 'True' for c in node['status']['conditions'])
assert not node['spec'].get('unschedulable')
assert not any(t.get('effect') in ('NoSchedule', 'NoExecute') for t in node['spec'].get('taints', []))
if not resume:
    (root / (component + '-workload.json')).write_text(json.dumps(saved, indent=2))
def abort(signum, frame):
    raise RuntimeError('Migration interrupted by signal ' + str(signum))

signal.signal(signal.SIGTERM, abort)
signal.signal(signal.SIGALRM, abort)
signal.alarm(12 * 3600)
try:
    if resume:
        assert get('configmap', lock_name)['data']['task_id'] == operation
        k('delete', 'pod', helper, '--ignore-not-found', '--wait=true', '--timeout=120s', timeout=140)
    else:
        create({'apiVersion': 'v1', 'kind': 'ConfigMap', 'metadata': {'name': lock_name}, 'data': {'task_id': operation}})
    locked = True
    note('PREPARE ' + source_node + ' -> ' + target + '; source PVC=' + source_pvc)
    existing = k('get', 'pvc', claim_name, '--ignore-not-found', '-o', 'json').strip()
    if not existing:
        spec = {key: copy.deepcopy(source_claim['spec'][key]) for key in ('accessModes', 'resources', 'storageClassName', 'volumeMode') if key in source_claim['spec']}
        create({'apiVersion': 'v1', 'kind': 'PersistentVolumeClaim', 'metadata': {'name': claim_name, 'labels': {'app.kubernetes.io/managed-by': 'cylism-manager', 'cylism.io/infrastructure': component}}, 'spec': spec})
    else:
        raise RuntimeError('Target PVC already exists; refusing to reuse a stale copy')
    create({'apiVersion': 'v1', 'kind': 'Pod', 'metadata': {'name': helper, 'labels': {'cylism.io/transfer-operation': operation}}, 'spec': {'nodeSelector': {'kubernetes.io/hostname': target}, 'restartPolicy': 'Never', 'automountServiceAccountToken': False, 'containers': [{'name': 'transfer', 'image': 'debian:bookworm-slim', 'imagePullPolicy': 'IfNotPresent', 'command': ['sleep', '86400'], 'resources': {'requests': {'cpu': '50m', 'memory': '64Mi'}, 'limits': {'cpu': '1', 'memory': '256Mi'}}, 'securityContext': {'allowPrivilegeEscalation': False, 'capabilities': {'drop': ['ALL'], 'add': ['CHOWN', 'DAC_OVERRIDE', 'FOWNER']}}, 'volumeMounts': [{'name': 'data', 'mountPath': '/data'}]}], 'volumes': [{'name': 'data', 'persistentVolumeClaim': {'claimName': claim_name}}]}})
    k('wait', '--for=condition=Ready', 'pod/' + helper, '--timeout=300s', timeout=330)
    target_claim = get('pvc', claim_name)
    target_pv = get('pv', target_claim['spec']['volumeName'])
    affinity = target_pv['spec']['nodeAffinity']['required']['nodeSelectorTerms']
    assert any(e['key'] == 'kubernetes.io/hostname' and e['values'] == [target] for term in affinity for e in term.get('matchExpressions', []))
    entries = k('exec', helper, '--', 'find', '/data', '-mindepth', '1', '-maxdepth', '1', '-print', '-quit').strip()
    assert resume or not entries, 'Target volume is non-empty: ' + entries
    size = int(subprocess.check_output(['du', '-sb', source_path], text=True).split()[0])
    available = int(k('exec', helper, '--', 'df', '-B1', '--output=avail', '/data').split()[-1])
    assert available >= size * 3 + 512 * 1024 * 1024
    assert shutil.disk_usage(root).free >= size * 2 + 512 * 1024 * 1024
    # Pre-pull the exact workload image before taking the source offline.
    puller = helper + '-image'
    image = source_spec['template']['spec']['containers'][0]['image']
    create({'apiVersion': 'v1', 'kind': 'Pod', 'metadata': {'name': puller}, 'spec': {'nodeSelector': {'kubernetes.io/hostname': target}, 'restartPolicy': 'Never', 'automountServiceAccountToken': False, 'containers': [{'name': 'pull', 'image': image, 'imagePullPolicy': 'IfNotPresent', 'command': ['/__cylism_image_pull_only__']}]}})
    deadline = time.monotonic() + 1800
    while not any(s.get('imageID') for s in get('pod', puller).get('status', {}).get('containerStatuses', [])):
        if time.monotonic() > deadline:
            raise RuntimeError('Image pre-pull timeout; source remains online')
        time.sleep(5)
    k('delete', 'pod', puller, '--wait=true', '--timeout=120s', timeout=150)
    note('PREFLIGHT OK source_bytes=' + str(size) + ' available_bytes=' + str(available))
    wait_running(source_node, source_pvc)
    stopped = True
    patch({'replicas': 0})
    wait_stopped()
    note('SOURCE STOPPED; copying into ' + claim_name)
    archive = root / (component + '.tar.gz')
    subprocess.run(['tar', '--numeric-owner', '-C', source_path, '-I', 'gzip -1', '-cpf', str(archive), '.'], check=True, timeout=1800)
    total = archive.stat().st_size
    note('ARCHIVE READY bytes=' + str(total))
    k('exec', helper, '--', 'mkdir', '/data/.cylism-transfer')
    expected = hashlib.sha256()
    with archive.open('rb') as src:
        index = 0
        while True:
            block = src.read(2 * 1024 * 1024)
            if not block:
                break
            expected.update(block)
            checksum = hashlib.sha256(block).hexdigest()
            dest = '/data/.cylism-transfer/part-%06d' % index
            command = 'set -eu; cat > ' + dest + '.tmp; echo "' + checksum + '  ' + dest + '.tmp" | sha256sum -c -; mv ' + dest + '.tmp ' + dest
            for attempt in range(1, 9):
                try:
                    result = subprocess.run(['kubectl', '-n', 'monitoring', 'exec', '-i', helper, '--', 'sh', '-c', command], input=block, capture_output=True, timeout=300)
                    if result.returncode:
                        raise RuntimeError(result.stderr.decode(errors='replace')[-1000:])
                    break
                except (subprocess.TimeoutExpired, RuntimeError) as error:
                    note('RETRY chunk=%d attempt=%d error=%s' % (index, attempt, error))
                    if attempt == 8:
                        raise
                    time.sleep(min(attempt * 5, 30))
            index += 1
            note('COPY bytes=%d/%d progress=%.1f%%' % (min(index * 2 * 1024 * 1024, total), total, min(index * 2 * 1024 * 1024, total) * 100 / total))
    actual = digest(['kubectl', '-n', 'monitoring', 'exec', helper, '--', 'bash', '-o', 'pipefail', '-c', 'cat /data/.cylism-transfer/part-* | sha256sum'])
    assert actual == expected.hexdigest(), 'Archive checksum mismatch'
    k('exec', helper, '--', 'bash', '-o', 'pipefail', '-c', 'cat /data/.cylism-transfer/part-* | tar --numeric-owner -xzpf - -C /data', timeout=1800)
    # Only remove staging files created by this job; original PVCs remain untouched.
    k('exec', helper, '--', 'rm', '-r', '/data/.cylism-transfer')
    command = "tar --sort=name --numeric-owner --mtime='UTC 1970-01-01' -cf - . | sha256sum"
    source_hash = digest(['bash', '-o', 'pipefail', '-c', 'cd ' + shlex.quote(source_path) + ' && ' + command])
    target_hash = digest(['kubectl', '-n', 'monitoring', 'exec', helper, '--', 'bash', '-o', 'pipefail', '-c', 'cd /data && ' + command])
    assert source_hash == target_hash, 'SHA-256 mismatch; refusing cutover'
    note('SHA256 VERIFIED ' + source_hash)
    k('delete', 'pod', helper, '--wait=true', '--timeout=120s', timeout=140)
    target_spec = copy.deepcopy(source_spec)
    target_spec['replicas'] = 1
    target_spec['template']['spec']['nodeSelector']['kubernetes.io/hostname'] = target
    for v in target_spec['template']['spec']['volumes']:
        if v['name'] == volume:
            v['persistentVolumeClaim']['claimName'] = claim_name
    patch(target_spec)
    pod_name = wait_running(target, claim_name)
    healthy = True
    note('SUCCEEDED pod=' + pod_name + ' node=' + target + ' PVC=' + claim_name + '; source PVC retained=' + source_pvc)
    (root / (component + '-result.json')).write_text(json.dumps({'status': 'succeeded', 'pod': pod_name, 'node': target, 'source_pvc': source_pvc, 'target_pvc': claim_name, 'sha256': source_hash, 'data_bytes': size}, indent=2))
except BaseException as error:
    signal.alarm(0)
    note('ERROR ' + str(error))
    if stopped and not healthy:
        note('ROLLING BACK')
        patch({'replicas': 0})
        wait_stopped()
        k('delete', 'pod', helper, '--ignore-not-found', '--wait=true', '--timeout=120s', timeout=140)
        patch(source_spec)
        wait_running(source_node, source_pvc)
        stopped = False
        note('ROLLED BACK; source ready')
    (root / (component + '-result.json')).write_text(json.dumps({'status': 'failed' if not stopped else 'rollback_failed', 'error': str(error)}, indent=2))
    raise
finally:
    signal.alarm(0)
    if locked and (healthy or not stopped):
        k('delete', 'pod', helper, '--ignore-not-found', '--wait=true', '--timeout=120s', timeout=140)
        k('delete', 'configmap', lock_name)
        note('Temporary resources cleaned; PVCs retained')

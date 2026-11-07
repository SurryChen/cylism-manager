# Invoked by mysqlsh, with credentials read only from the mounted task Secret.
# MySQL Shell can embed credentials in errors. Discard its raw output at the OS
# descriptor level and emit only our allowlisted machine-readable events.
import os, sys, json, time, shutil, threading
from pathlib import Path
_event_fd = os.dup(1)
_null = os.open(os.devnull, os.O_WRONLY)
os.dup2(_null, 1)
os.dup2(_null, 2)
import mysqlsh
shell = mysqlsh.globals.shell
util = mysqlsh.globals.util

def emit(value):
    os.write(_event_fd, ('CYLISM_EVENT:' + json.dumps(value, ensure_ascii=True) + '\n').encode())

class Rejected(Exception):
    def __init__(self, code): self.code = code

def require(condition, code):
    if not condition: raise Rejected(code)

def connect(ep):
    return shell.open_session({'scheme': 'mysql', 'host': ep['host'], 'port': ep['port'], 'user': ep['user'], 'password': ep['password'], 'ssl-mode': 'REQUIRED'})

def global_connect(ep):
    shell.connect({'scheme': 'mysql', 'host': ep['host'], 'port': ep['port'], 'user': ep['user'], 'password': ep['password'], 'ssl-mode': 'REQUIRED'})
    return mysqlsh.globals.session

SYSTEM = ['mysql', 'sys', 'information_schema', 'performance_schema']
def schemas(session):
    return [row[0] for row in session.run_sql('SHOW DATABASES').fetch_all() if row[0].lower() not in SYSTEM]

def scalar(session, query): return session.run_sql(query).fetch_one()[0]

def objects(session, selected):
    queries = {
        'tables': 'SELECT TABLE_NAME, TABLE_TYPE FROM information_schema.TABLES WHERE TABLE_SCHEMA=? ORDER BY TABLE_NAME',
        'routines': 'SELECT ROUTINE_NAME, ROUTINE_TYPE FROM information_schema.ROUTINES WHERE ROUTINE_SCHEMA=? ORDER BY ROUTINE_NAME, ROUTINE_TYPE',
        'triggers': 'SELECT TRIGGER_NAME FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=? ORDER BY TRIGGER_NAME',
        'events': 'SELECT EVENT_NAME FROM information_schema.EVENTS WHERE EVENT_SCHEMA=? ORDER BY EVENT_NAME',
    }
    return {schema: {kind: [list(row) for row in session.run_sql(query, [schema]).fetch_all()] for kind, query in queries.items()} for schema in selected}

def replica(session, channel):
    return next((row for row in replica_channels(session) if row.get('Channel_Name', '') == channel), None)

def replica_channels(session):
    result = session.run_sql('SHOW REPLICA STATUS')
    return [dict(zip(result.get_column_names(), row)) for row in result.fetch_all()]

def replication_observation(session, row):
    # No source credentials or raw SQL/error messages may enter task results.
    return {'channel': row.get('Channel_Name', ''),
            'io_running': row.get('Replica_IO_Running') == 'Yes',
            'sql_running': row.get('Replica_SQL_Running') == 'Yes',
            'lag_seconds': row.get('Seconds_Behind_Source'),
            'io_errno': int(row.get('Last_IO_Errno') or 0),
            'sql_errno': int(row.get('Last_SQL_Errno') or 0),
            'read_only': bool(scalar(session, 'SELECT @@global.read_only')),
            'super_read_only': bool(scalar(session, 'SELECT @@global.super_read_only')),
            'schemas': row.get('Replicate_Do_DB', ''),
            'tables': row.get('Replicate_Wild_Do_Table', '')}

def discover_replication(source, target):
    global stage
    stage = 'source_identity'
    source_uuid = str(scalar(source, 'SELECT @@server_uuid'))
    stage = 'target_identity'
    require(source_uuid != str(scalar(target, 'SELECT @@server_uuid')), 'same_server')
    stage = 'target_channels'
    rows = replica_channels(target)
    require(bool(rows), 'replication_missing')
    # The platform currently models one upstream channel per target. Never
    # silently adopt one channel of a multi-source replica.
    require(len(rows) == 1, 'replication_multiple_channels')
    row = rows[0]
    require(str(row.get('Source_UUID', '')).lower() == source_uuid.lower(), 'replication_source_mismatch')
    stage = 'target_observation'
    result = replication_observation(target, row)
    result['source_verified'] = True
    return result

def progress():
    while not done.wait(15):
        total = sum(p.stat().st_size for p in Path('/work/dump').rglob('*') if p.is_file()) if Path('/work/dump').exists() else 0
        emit({'progress_bytes': total})

def quote_literal(value):
    # NO_BACKSLASH_ESCAPES is set for the control session below.
    return "'" + value.replace("'", "''") + "'"

kind = ''
stage = 'initialization'
try:
    req = json.load(open('/run/task/input.json'))
    phase = int(os.environ['CYLISM_PHASE'])
    kind = req['kind']
    channel = req['channel']
    dump = '/work/dump'
    done = threading.Event()
    threading.Thread(target=progress, daemon=True).start()
    if phase == -1:
        # Cancellation audits only observe; no STOP/START or promotion.
        session = connect(req['target'])
        row = replica(session, channel)
        emit({'result': {'channel_present': row is not None, 'io_running': bool(row and row['Replica_IO_Running'] == 'Yes'), 'sql_running': bool(row and row['Replica_SQL_Running'] == 'Yes')}})
    elif kind == 'inspect':
        session = connect(req['source'])
        emit({'result': {'version': str(scalar(session, 'SELECT VERSION()')), 'schemas': schemas(session), 'read_only': bool(scalar(session, 'SELECT @@global.read_only'))}})
    elif kind == 'switch':
        for conn in req['connections']:
            ep = dict(req['target'], user=conn['username'], password=conn['password'])
            session = connect(ep)
            session.set_current_schema(conn['database'])
            session.run_sql('SELECT 1').fetch_one()
    elif kind == 'replication_discover':
        stage = 'source_connect'
        source = connect(req['source'])
        stage = 'target_connect'
        target = connect(req['target'])
        emit({'result': discover_replication(source, target)})
    elif kind in ('detach', 'replication_check'):
        session = connect(req['target'])
        row = replica(session, channel)
        if kind == 'detach':
            if row:
                session.run_sql("SET SESSION sql_mode = 'NO_BACKSLASH_ESCAPES'")
                session.run_sql('STOP REPLICA FOR CHANNEL ' + quote_literal(channel))
                session.run_sql('RESET REPLICA ALL FOR CHANNEL ' + quote_literal(channel))
            # No automatic promotion: detach leaves super_read_only in place.
        else:
            require(row is not None, 'replication_missing')
            emit({'result': replication_observation(session, row)})
    elif kind in ('migrate', 'replicate'):
        if phase == 0:
            source = connect(req['source']); target = connect(req['target'])
            require(scalar(source, 'SELECT @@server_uuid') != scalar(target, 'SELECT @@server_uuid'), 'same_server')
            # Start with MySQL 8.0/8.4, same major series to avoid silent
            # incompatible SQL downgrades. Broader version support needs a
            # separate compatibility check rather than ignoreVersion=true.
            sv = str(scalar(source, 'SELECT VERSION()')).split('.')[:2]
            tv = str(scalar(target, 'SELECT VERSION()')).split('.')[:2]
            require(sv == tv and sv in (['8','0'], ['8','4']), 'version_mismatch')
            source_schemas = schemas(source); target_schemas = schemas(target)
            selected = req['schemas'] if kind == 'migrate' else source_schemas
            require(bool(selected) and all(name in source_schemas for name in selected), 'schema_missing')
            require(not any(name in target_schemas for name in selected), 'target_schema_exists')
            require(not target.run_sql('SHOW REPLICA STATUS').fetch_one(), 'target_has_replication')
            require(bool(scalar(target, 'SELECT @@global.local_infile')), 'local_infile_required')
            require(not scalar(target, 'SELECT @@global.read_only'), 'target_read_only')
            if kind == 'replicate':
                require(not target_schemas, 'target_not_empty')
                require(scalar(source, 'SELECT @@global.gtid_mode') == 'ON' and scalar(target, 'SELECT @@global.gtid_mode') == 'ON', 'gtid_required')
                require(bool(scalar(source, 'SELECT @@global.log_bin')), 'binlog_required')
                require(not scalar(target, 'SELECT @@global.gtid_executed'), 'target_gtid_not_empty')
                # Require a pre-created replication-only credential. Never
                # reuse the administration password in CHANGE SOURCE.
                connect(dict(req['source'], user=req['replication_user'], password=req['replication_password'])).close()
            Path('/work').mkdir(exist_ok=True)
            target_version = str(scalar(target, 'SELECT VERSION()')).split('-')[0].split('.')[:3]
            selection = {'schemas': selected, 'target_version': '.'.join(target_version)}
            if kind == 'migrate': selection['objects'] = objects(source, selected)
            json.dump(selection, open('/work/selection.json', 'w'))
        elif phase == 1:
            # Re-observing an already-completed dump is safe. Partial dumps are
            # task-owned files; no target database is changed during this phase.
            if not Path(dump + '/@.done.json').exists():
                if Path(dump).exists(): shutil.rmtree(dump)
                source = global_connect(req['source'])
                selection = json.load(open('/work/selection.json'))
                selected = selection['schemas']
                opts = {'consistent': True, 'threads': 2, 'showProgress': False, 'targetVersion': selection['target_version']}
                if kind == 'replicate':
                    util.dump_instance(dump, dict(opts, users=False))
                else:
                    util.dump_schemas(selected, dump, opts)
            meta = json.load(open(dump + '/@.json'))
            require(not meta.get('gtidExecutedInconsistent', False), 'inconsistent_gtid')
            if kind == 'replicate': require(bool(meta.get('gtidExecuted')), 'snapshot_gtid_missing')
        elif phase == 2:
            target = global_connect(req['target'])
            # The loader's own persistent progress journal permits recovery,
            # but automatic Job retries are disabled. Never resetProgress or
            # ignoreExistingObjects; existing unrelated data is never dropped.
            opts = {'threads': 2, 'showProgress': False, 'progressFile': '/work/load-progress.json'}
            if kind == 'replicate':
                opts.update({'skipBinlog': True, 'updateGtidSet': 'replace'})
            util.load_dump(dump, opts)
        elif kind == 'migrate' and phase == 3:
            target = connect(req['target'])
            selection = json.load(open('/work/selection.json'))
            selected = selection['schemas']
            # Dump completion + successful load is required, then verify every
            # selected object against the inventory captured while writes were stopped.
            for name in selected:
                require(name in schemas(target), 'load_schema_missing')
            require(objects(target, selected) == selection['objects'], 'load_objects_mismatch')
            emit({'result': {'schemas': selected, 'snapshot_loaded': True, 'application_switch_required': True}})
        elif kind == 'replicate' and phase == 3:
            target = connect(req['target'])
            target.run_sql("SET SESSION sql_mode = 'NO_BACKSLASH_ESCAPES'")
            row = replica(target, channel)
            if not row:
                require(not target.run_sql('SHOW REPLICA STATUS').fetch_one(), 'target_has_replication')
                target.run_sql('SET GLOBAL super_read_only = ON')
                # Persist across MySQL restarts, rather than only setting the
                # current process. System variables are not copied from source.
                target.run_sql('SET PERSIST super_read_only = ON')
                target.run_sql('CHANGE REPLICATION SOURCE TO SOURCE_HOST=' + quote_literal(req['source']['host']) + ', SOURCE_PORT=' + str(int(req['source']['port'])) + ', SOURCE_USER=' + quote_literal(req['replication_user']) + ', SOURCE_PASSWORD=' + quote_literal(req['replication_password']) + ", SOURCE_AUTO_POSITION=1, SOURCE_SSL=1 FOR CHANNEL '" + channel + "'")
            target.run_sql("START REPLICA FOR CHANNEL '" + channel + "'")
        elif kind == 'replicate' and phase == 4:
            target = connect(req['target'])
            while True:
                row = replica(target, channel)
                require(row is not None, 'replication_missing')
                require(not row.get('Last_IO_Errno') and not row.get('Last_SQL_Errno'), 'replication_thread_error')
                if row['Replica_IO_Running'] == 'Yes' and row['Replica_SQL_Running'] == 'Yes':
                    emit({'result': replication_observation(target, row)})
                    break
                emit({'waiting': 'replication_connecting'})
                time.sleep(5)
    else:
        raise Rejected('unsupported_kind')
    done.set()
    emit({'done': True})
except BaseException as error:
    # Error codes and allowlisted rejection reasons are useful; raw SQL,
    # usernames/passwords and connector error messages never leave this process.
    code = error.code if isinstance(error, Rejected) else 'mysql_command_failed'
    if kind == 'replication_discover' and code == 'mysql_command_failed':
        errno = getattr(error, 'code', None)
        errno = str(errno) if isinstance(errno, int) and 0 <= errno <= 9999 else '0'
        code += '@' + stage + ':' + errno
    emit({'error': code})
    sys.exit(1)

#!/usr/bin/env python3
"""Offline evaluation of a production Vela binary against synthetic source."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import select
import shutil
import subprocess
import sys
import tempfile
import time

from golden_score import aggregate, compare, score_set

ROOT = Path(__file__).resolve().parent.parent
CORPUS = ROOT / "tests/golden"


def corpus_id():
    # Versioned identity includes the generated-fixture recipe in this script.
    h = hashlib.sha256(b'vela-golden-recipe-v2\0')
    files = [(str(p.relative_to(CORPUS)), p) for p in CORPUS.rglob('*') if p.is_file()]
    files += [('harness/' + p.name, p) for p in (Path(__file__).resolve(), ROOT / 'scripts/golden_score.py')]
    for name, path in sorted(files):
        h.update(name.encode() + b'\0' + path.read_bytes() + b'\0')
    return 'sha256-v2:' + h.hexdigest()


def isolated_env(home):
    # Only system tools are visible to Vela; no inherited provider credentials,
    # configuration, driver binaries, or proxy settings are passed through.
    return {"HOME": str(home), "XDG_CONFIG_HOME": str(home / 'config'),
            "XDG_CACHE_HOME": str(home / 'cache'), "PATH": str(home / 'empty-path'),
            "GOPROXY": "off", "GOSUMDB": "off", "GOENV": "off",
            "GOTOOLCHAIN": "local", "GOWORK": "off", "TMPDIR": str(home)}


def build_cli(binary, env):
    go = shutil.which('go')
    if not go:
        raise RuntimeError('go executable not found')
    # Cached modules/build objects are read from the developer's Go cache; no
    # network or inherited Go configuration is available to the build.
    discovery_env = dict(env, HOME=str(Path.home()), PATH=str(Path(go).resolve().parent) + ':/usr/bin:/bin')
    discovery_env.pop('XDG_CACHE_HOME', None)
    build_env = dict(env, PATH=discovery_env['PATH'])
    for name in ('GOCACHE', 'GOMODCACHE'):
        build_env[name] = subprocess.check_output([go, 'env', name], text=True, env=discovery_env, cwd=ROOT, timeout=30).strip()
    subprocess.run([go, 'build', '-o', str(binary), './cmd/vela'], cwd=ROOT,
                   env=build_env, check=True, timeout=300)


class MCP:
    def __init__(self, binary, root, env):
        self.err = tempfile.TemporaryFile(mode='w+')
        try:
            self.p = subprocess.Popen([str(binary), 'serve', '--mcp'], cwd=root, env=env,
                                      stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                      stderr=self.err, bufsize=0)
        except Exception:
            self.err.close()
            raise
        self.seq = 0
        self.buffer = b''
        try:
            self.request('initialize', {'protocolVersion': '2024-11-05', 'capabilities': {},
                                        'clientInfo': {'name': 'golden-eval', 'version': '1'}})
            self.send({'jsonrpc': '2.0', 'method': 'notifications/initialized'})
        except Exception:
            self.close()
            raise

    def send(self, obj):
        self.p.stdin.write((json.dumps(obj) + '\n').encode())
        self.p.stdin.flush()

    def request(self, method, params):
        self.seq += 1
        self.send({'jsonrpc': '2.0', 'id': self.seq, 'method': method, 'params': params})
        deadline = time.monotonic() + 30
        while True:
            remaining = deadline - time.monotonic()
            while b'\n' not in self.buffer:
                remaining = deadline - time.monotonic()
                if remaining <= 0 or not select.select([self.p.stdout], [], [], remaining)[0]:
                    raise TimeoutError('MCP response timeout')
                chunk = os.read(self.p.stdout.fileno(), 65536)
                if not chunk:
                    raise RuntimeError('MCP server exited')
                self.buffer += chunk
                if len(self.buffer) > 8 * 1024 * 1024:
                    raise ValueError('MCP response exceeds 8 MiB')
            line, self.buffer = self.buffer.split(b'\n', 1)
            message = json.loads(line)
            if not isinstance(message, dict):
                raise ValueError('invalid JSON-RPC response shape')
            if message.get('id') == self.seq:
                if 'error' in message:
                    raise RuntimeError(str(message['error']))
                result = message.get('result')
                if not isinstance(result, dict):
                    raise ValueError('invalid JSON-RPC result shape')
                return result

    def close(self):
        if self.p.poll() is None:
            self.p.terminate()
            try:
                self.p.wait(timeout=5)
            except subprocess.TimeoutExpired:
                self.p.kill()
                self.p.wait()
        self.p.stdin.close()
        self.p.stdout.close()
        self.err.close()


def values(result, field):
    if not isinstance(result, dict):
        raise ValueError('invalid MCP result shape')
    if result.get('isError'):
        raise RuntimeError('MCP tool error: ' + str(result))
    if field == 'tools':
        rows, keys = result.get('tools'), ('name',)
    else:
        data = result.get('structuredContent')
        if not isinstance(data, dict):
            raise RuntimeError('missing structuredContent: ' + str(result))
        status = data.get('status')
        if not isinstance(status, str):
            raise ValueError('invalid status shape')
        if field == 'status':
            return [[status]]
        if field == 'freshness':
            if status != 'ok':
                raise RuntimeError('unexpected status: ' + status)
            fresh = data.get('freshness')
            if not isinstance(fresh, dict) or not isinstance(fresh.get('status'), str):
                raise ValueError('invalid freshness shape')
            return [[fresh['status']]]
        wanted = 'ambiguous' if field == 'ambiguous_ids' else 'ok'
        if status != wanted:
            raise RuntimeError('unexpected status: ' + status)
        if field in ('labels', 'ambiguous_ids'):
            rows, keys = data.get('resolved_subjects', []), (('label',) if field == 'labels' else ('id',))
        elif field in ('facts', 'path'):
            rows, keys = data.get('facts', []), ('subject', 'predicate', 'object')
        else:
            raise ValueError('unknown field ' + field)
    if not isinstance(rows, list) or any(not isinstance(row, dict) or any(not isinstance(row.get(k), str) for k in keys) for row in rows):
        raise ValueError('invalid ' + field + ' shape')
    extracted = [[row[k] for k in keys] for row in rows]
    if field == 'path':
        return [['path'] + [item for fact in extracted for item in fact]] if extracted else []
    return extracted


def category(case):
    if case['tool'] == 'manifest':
        return 'inventory'
    if case['field'] in ('status', 'freshness') and case['tool'] == 'status':
        return 'lifecycle'
    if case['tool'] == 'tools/list':
        return 'discovery'
    return 'quality'


def summary(results):
    return {'total': len(results), 'passed': sum(c['pass'] for c in results),
            'score': aggregate(c['score'] for c in results)}


def run(binary, work, env, cases):
    results = []
    for fixture in sorted({c['fixture'] for c in cases}):
        root = work / fixture
        server = quality_server = None
        startup_error = None
        build_error = None
        try:
            shutil.copytree(CORPUS / 'fixtures' / fixture, root)
            if fixture == 'inventory':
                (root / 'node_modules').mkdir()
                (root / 'node_modules' / 'go.mod').symlink_to('../main.go')
            try:
                server = MCP(binary, root, env)
            except (ValueError, RuntimeError, TimeoutError, OSError) as exc:
                startup_error = 'graphless initialization failed: ' + str(exc)
            for phase in ('missing', 'published', 'stale', 'rebuilt'):
                selected = [c for c in cases if c['fixture'] == fixture and c['phase'] == phase]
                if phase in ('published', 'rebuilt'):
                    try:
                        proc = subprocess.run([str(binary), 'build', str(root)], cwd=root, env=env,
                                              stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=120)
                        build_error = None if proc.returncode == 0 else 'fixture build failed: ' + proc.stderr.decode(errors='replace')[-2000:]
                    except (OSError, subprocess.TimeoutExpired) as exc:
                        build_error = 'fixture build failed: ' + str(exc)
                    if phase == 'published' and startup_error and not build_error:
                        try:
                            quality_server = MCP(binary, root, env)
                        except (ValueError, RuntimeError, TimeoutError, OSError) as exc:
                            startup_error += '; post-publish initialization failed: ' + str(exc)
                if phase == 'stale':
                    source = root / ('main.go' if fixture in ('bound', 'inventory') else ('service.py' if fixture == 'dev' else 'catalog.py'))
                    with source.open('a') as f:
                        f.write('\n// changed\n' if fixture in ('bound', 'inventory') else '\n# changed\n')
                for case in selected:
                    start = time.monotonic()
                    error = None
                    actual = []
                    try:
                        if phase != 'missing' and build_error:
                            raise RuntimeError(build_error)
                        if case['tool'] == 'manifest':
                            manifest = root / '.vela' / '.current' / 'manifest.json'
                            if not manifest.exists():
                                manifest = root / '.vela' / 'manifest.json'
                            data = json.loads(manifest.read_text())
                            if not isinstance(data, dict) or not data.get('inventory_complete') or not isinstance(data.get('files'), list):
                                raise RuntimeError('manifest inventory incomplete or unavailable')
                            if any(not isinstance(f, dict) or not isinstance(f.get('path'), str) for f in data['files']):
                                raise ValueError('invalid manifest files shape')
                            actual = [[f['path']] for f in data['files']]
                        else:
                            active = server if category(case) == 'lifecycle' or phase == 'missing' else (server or quality_server)
                            if active is None:
                                raise RuntimeError(startup_error or 'MCP session unavailable')
                            method = 'tools/list' if case['tool'] == 'tools/list' else 'tools/call'
                            params = {} if method == 'tools/list' else {'name': case['tool'], 'arguments': case['args']}
                            actual = values(active.request(method, params), case['field'])
                    except (ValueError, RuntimeError, TimeoutError, OSError, KeyError) as exc:
                        error = str(exc)
                    score = score_set(case['expected'], actual)
                    results.append({'id': case['id'], 'fixture': fixture, 'category': category(case),
                                    'expected': case['expected'], 'actual': actual,
                                    'score': score, 'pass': score['exact'] and error is None,
                                    'seconds': time.monotonic() - start, 'error': error})
        finally:
            if server:
                server.close()
            if quality_server:
                quality_server.close()
    return {'corpus': corpus_id(), 'binary_sha256': hashlib.sha256(Path(binary).read_bytes()).hexdigest(), 'cases': results, 'summary': summary(results),
            'by_fixture': {v: summary([c for c in results if c['fixture'] == v]) for v in sorted({c['fixture'] for c in results})},
            'by_category': {v: summary([c for c in results if c['category'] == v]) for v in sorted({c['category'] for c in results})}}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, help='use an existing Vela binary instead of building')
    parser.add_argument('--output', type=Path, help='write report JSON (default stdout)')
    parser.add_argument('--compare', nargs=2, type=Path, metavar=('BASE', 'CANDIDATE'))
    args = parser.parse_args()
    if args.compare:
        report = {'changes': compare(*(json.loads(p.read_text()) for p in args.compare))}
    else:
        cases = json.loads((CORPUS / 'cases.json').read_text())
        if len({c['id'] for c in cases}) != len(cases):
            raise ValueError('duplicate case ID')
        with tempfile.TemporaryDirectory(prefix='vela-golden-') as tmp:
            work = Path(tmp)
            env = isolated_env(work)
            (work / 'empty-path').mkdir()
            binary = args.binary.resolve() if args.binary else work / 'vela'
            if not args.binary:
                build_cli(binary, env)
            report = run(binary, work, env, cases)
    text = json.dumps(report, indent=2) + '\n'
    if args.output:
        args.output.write_text(text)
    else:
        sys.stdout.write(text)


if __name__ == '__main__':
    main()

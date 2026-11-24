from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import golden_eval
from golden_eval import values
from golden_score import score_set


class RunnerScoringTests(unittest.TestCase):
    def test_path_order_and_typed_relationships(self):
        def result(facts):
            return {'structuredContent': {'status': 'ok', 'facts': [dict(subject=a, predicate=b, object=c)
                                                    for a, b, c in facts]}}
        forward = values(result([('a', 'calls', 'b'), ('b', 'uses', 'c')]), 'path')
        reverse = values(result([('b', 'uses', 'c'), ('a', 'calls', 'b')]), 'path')
        wrong_kind = values(result([('a', 'uses', 'b'), ('b', 'uses', 'c')]), 'path')
        self.assertFalse(score_set(forward, reverse)['exact'])
        self.assertFalse(score_set(forward, wrong_kind)['exact'])

    def test_text_only_does_not_silently_pass_empty_facts(self):
        with self.assertRaisesRegex(RuntimeError, 'missing structuredContent'):
            values({'content': [{'type': 'text', 'text': 'no dependencies'}]}, 'facts')

    def test_non_ok_and_malformed_are_rejected(self):
        for result, field in [({'structuredContent': {'status': 'unresolved'}}, 'facts'),
                              ({'structuredContent': {'status': 'ok', 'facts': None}}, 'facts'),
                              ({'structuredContent': {'status': 'ok', 'freshness': None}}, 'freshness'),
                              ({'tools': [None]}, 'tools'), ([], 'status')]:
            with self.subTest(result=result), self.assertRaises((ValueError, RuntimeError)):
                values(result, field)

    def test_freshness_requires_ok_status(self):
        for status in ('unavailable', 'error'):
            with self.subTest(status=status), self.assertRaisesRegex(RuntimeError, 'unexpected status'):
                values({'structuredContent': {'status': status, 'freshness': {'status': 'fresh'}}}, 'freshness')
        self.assertEqual(values({'structuredContent': {'status': 'ok', 'freshness': {'status': 'fresh'}}}, 'freshness'), [['fresh']])

    def test_tool_inventory_is_exact_set(self):
        actual = values({'tools': [{'name': 'status'}, {'name': 'lookup'}]}, 'tools')
        self.assertTrue(score_set([['lookup'], ['status']], actual)['exact'])
        self.assertFalse(score_set([['status']], actual)['exact'])


class RunnerFailureTests(unittest.TestCase):
    def test_malformed_case_does_not_stop_following_case(self):
        cases = [dict(id=ident, fixture='dev', phase='published', tool='lookup', args={}, field='labels', expected=[['ok']]) for ident in ('bad', 'good')]
        class Server:
            count = 0
            def request(self, method, params):
                self.count += 1
                if self.count == 1:
                    return {'structuredContent': {'status': 'ok', 'resolved_subjects': None}}
                return {'structuredContent': {'status': 'ok', 'resolved_subjects': [{'label': 'ok'}]}}
            def close(self):
                pass
        with tempfile.TemporaryDirectory() as tmp, patch.object(golden_eval, 'MCP', return_value=Server()), \
             patch.object(golden_eval.subprocess, 'run', return_value=subprocess.CompletedProcess([], 0, b'', b'')):
            binary = Path(tmp) / 'binary'
            binary.write_bytes(b'test')
            results = golden_eval.run(binary, Path(tmp), {}, cases)['cases']
        self.assertFalse(results[0]['pass'])
        self.assertIn('shape', results[0]['error'])
        self.assertTrue(results[1]['pass'])

    def test_invalid_matching_results_are_case_errors_and_do_not_stop_run(self):
        cases = [dict(id=ident, fixture='dev', phase='published', tool='lookup', args={}, field='labels', expected=[['ok']]) for ident in ('bad', 'good')]
        for response in ('{"id": 1}', '{"id": 1, "result": null}',
                         '{"id": 1, "result": []}', '{"id": 1, "result": "bad"}'):
            with self.subTest(response=response), tempfile.TemporaryDirectory() as tmp:
                server = object.__new__(golden_eval.MCP)
                server.seq = 0
                server.send = lambda obj: None
                server.close = lambda: None
                server.buffer = (response + '\n' +
                    '{"id": 2, "result": {"structuredContent": {"status": "ok", "resolved_subjects": [{"label": "ok"}]}}}\n').encode()
                with patch.object(golden_eval, 'MCP', return_value=server), \
                     patch.object(golden_eval.subprocess, 'run', return_value=subprocess.CompletedProcess([], 0, b'', b'')):
                    binary = Path(tmp) / 'binary'
                    binary.write_bytes(b'test')
                    results = golden_eval.run(binary, Path(tmp), {}, cases)['cases']
                self.assertFalse(results[0]['pass'])
                self.assertIn('invalid JSON-RPC result shape', results[0]['error'])
                self.assertTrue(results[1]['pass'])

    def test_failed_initialization_closes_process(self):
        with patch.object(golden_eval.subprocess, 'Popen') as popen, \
             patch.object(golden_eval.MCP, 'request', side_effect=RuntimeError('failed')):
            popen.return_value.poll.return_value = None
            with self.assertRaises(RuntimeError):
                golden_eval.MCP('binary', '.', {})
            popen.return_value.terminate.assert_called_once()
            popen.return_value.wait.assert_called_once()
            self.assertTrue(popen.return_value.stdin.close.called)
            self.assertTrue(popen.return_value.stdout.close.called)

    def test_partial_line_is_bounded_by_timeout(self):
        mcp = object.__new__(golden_eval.MCP)
        mcp.p = unittest.mock.MagicMock()
        mcp.seq = 0
        mcp.buffer = b''
        with patch.object(golden_eval.select, 'select', side_effect=[([mcp.p.stdout], [], []), ([], [], [])]), \
             patch.object(golden_eval.os, 'read', return_value=b'{"id":'):
            with self.assertRaises(TimeoutError):
                mcp.request('initialize', {})

    def test_failed_start_and_build_preserve_independent_cases(self):
        cases = [
            {'id': 'missing', 'fixture': 'dev', 'phase': 'missing', 'tool': 'status', 'args': {}, 'field': 'status', 'expected': [['unavailable']]},
            {'id': 'fresh', 'fixture': 'dev', 'phase': 'published', 'tool': 'status', 'args': {}, 'field': 'freshness', 'expected': [['fresh']]},
            {'id': 'quality', 'fixture': 'dev', 'phase': 'published', 'tool': 'lookup', 'args': {}, 'field': 'labels', 'expected': [['ok']]},
            {'id': 'build-fail', 'fixture': 'holdout', 'phase': 'published', 'tool': 'lookup', 'args': {}, 'field': 'labels', 'expected': [['ok']]},
        ]
        class Server:
            def request(self, method, params):
                return {'structuredContent': {'status': 'ok', 'resolved_subjects': [{'label': 'ok'}]}}
            def close(self):
                pass
        def start(binary, root, env):
            if root.name == 'dev' and not (root / 'published').exists():
                raise RuntimeError('graphless exit')
            return Server()
        def build(args, cwd, **kwargs):
            if cwd.name == 'holdout':
                return subprocess.CompletedProcess(args, 1, b'', b'failed build')
            (cwd / 'published').touch()
            return subprocess.CompletedProcess(args, 0, b'', b'')
        with tempfile.TemporaryDirectory() as tmp, \
             patch.object(golden_eval, 'MCP', side_effect=start), \
             patch.object(golden_eval.subprocess, 'run', side_effect=build):
            binary = Path(tmp) / 'binary'
            binary.write_bytes(b'test')
            report = golden_eval.run(binary, Path(tmp), {}, cases)
        results = {c['id']: c for c in report['cases']}
        self.assertEqual(set(results), {c['id'] for c in cases})
        self.assertFalse(results['missing']['pass'])
        self.assertFalse(results['fresh']['pass'])
        self.assertIn('graphless', results['fresh']['error'])
        self.assertTrue(results['quality']['pass'])
        self.assertIn('fixture build failed', results['build-fail']['error'])
        self.assertEqual(report['by_fixture']['dev']['total'], 3)
        self.assertEqual(report['by_category']['quality']['total'], 2)


if __name__ == '__main__':
    unittest.main()

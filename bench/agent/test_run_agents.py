import json
import os
import subprocess
import pathlib
import tempfile
import unittest
from types import SimpleNamespace
from unittest.mock import patch

import run_agents as runner
import feedback_loop


class SelectionTests(unittest.TestCase):
    def test_categories_are_distributed_across_repositories(self):
        categories = ['direct', 'messy_pt', 'voice_mode', 'negative']
        cases = [{'id': f'{repo}-{category}', 'repo': repo, 'category': category}
                 for repo in ('a', 'b', 'c', 'd') for category in categories]
        with tempfile.TemporaryDirectory() as tmp:
            path = pathlib.Path(tmp, 'manifest.json')
            path.write_text(json.dumps({'repos': {}, 'cases': cases}))
            _, selected = runner.load_cases(path, categories, 1)
        self.assertEqual([c['repo'] for c in selected], ['a', 'b', 'c', 'd'])
        self.assertEqual([c['category'] for c in selected], categories)


class ScheduleTests(unittest.TestCase):
    def test_adjacent_pairs_alternate_order(self):
        cases = [{'id': str(i)} for i in range(4)]
        jobs = runner.schedule(cases, [('claude', 'haiku')], 1)
        self.assertEqual([job[3] for job in jobs], ['OFF', 'ON', 'ON', 'OFF', 'OFF', 'ON', 'ON', 'OFF'])
        for i in range(0, len(jobs), 2):
            self.assertEqual(jobs[i][0]['id'], jobs[i+1][0]['id'])
        self.assertEqual(jobs, runner.schedule(cases, [('claude', 'haiku')], 1))


class AnswerTests(unittest.TestCase):
    def test_valid_paths_and_negative(self):
        self.assertEqual(runner.answer_files('result: {"files": ["./src/a.go", ".config"]}'),
                         ['src/a.go', '.config'])
        self.assertEqual(runner.answer_files('{"files": []}'), [])
        negative = {'category': 'negative'}
        self.assertTrue(runner.judge(negative, [])['success'])
        self.assertIsNone(runner.attention(negative, [], '', '/tmp')['gold_in_ctx'])

    def test_invalid_answers_are_not_abstentions(self):
        for answer in ('{"files": [42]}', '{"files": ["../a"]}',
                       '{"files": ["/a"]}', '{"files": [""]}',
                       '{"files": ["a", "b", "c", "d", "e", "f"]}', 'no JSON'):
            with self.subTest(answer=answer):
                self.assertIsNone(runner.answer_files(answer))
                self.assertFalse(runner.judge({'category': 'negative', 'relevant': []},
                                              runner.answer_files(answer))['success'])


class ClientOutputTests(unittest.TestCase):
    def test_claude_usage_includes_cached_input(self):
        output = json.dumps({'type': 'result', 'result': '{"files": ["a.go"]}',
                             'usage': {'input_tokens': 10, 'cache_read_input_tokens': 20,
                                       'cache_creation_input_tokens': 3, 'output_tokens': 4},
                             'total_cost_usd': 0.01})
        parsed = runner.parse_output('claude', output)
        self.assertEqual((parsed['tokens_in'], parsed['tokens_out'], parsed['cost_usd']), (33, 4, 0.01))
        self.assertEqual(runner.answer_files(parsed['text']), ['a.go'])

    def test_codex_events_and_antigravity_response(self):
        events = [{'type': 'item.completed', 'item': {'type': 'command_execution'}},
                  {'type': 'item.completed', 'item': {'type': 'agent_message', 'text': '{"files": []}'}},
                  {'type': 'turn.completed', 'usage': {'input_tokens': 10, 'output_tokens': 2}}]
        parsed = runner.parse_output('codex', '\n'.join(map(json.dumps, events)))
        self.assertEqual((parsed['tool_calls'], parsed['tokens_in'], parsed['tokens_out']), (1, 10, 2))
        self.assertEqual(runner.answer_files(parsed['text']), [])
        parsed = runner.parse_output('agy', json.dumps({'response': '{"files": ["a.go"]}',
                                                         'usage': {'prompt_tokens': 12, 'completion_tokens': 3}}))
        self.assertEqual((parsed['tokens_in'], parsed['tokens_out']), (12, 3))
        self.assertEqual(runner.answer_files(parsed['text']), ['a.go'])


class FeedbackTargetsTests(unittest.TestCase):
    def test_siblings_share_repo_and_gold_including_voice(self):
        def case(cid, repo, path, category):
            return {'id': cid, 'repo': repo, 'category': category, 'relevant': [{'path': path}]}
        cases = [case('R01-1', 'r', 'a', 'direct'), case('R01-3', 'r', 'b', 'messy_pt'),
                 case('R01-4', 'r', 'a', 'voice_mode'), case('R02-1', 's', 'a', 'direct')]
        targets = feedback_loop.targets(cases, 10)
        self.assertEqual([(a['id'], [b['id'] for b in bs]) for a, bs in targets],
                         [('R01-1', ['R01-4'])])

    def test_feedback_experiment_selects_v2_explicitly(self):
        with patch.object(feedback_loop.subprocess, 'run', return_value=SimpleNamespace(stdout='{}')) as run:
            feedback_loop.explain('sidecar', 'home', 'repo', 'prompt')
        self.assertIn('--engine', run.call_args.args[0])
        self.assertIn('v2', run.call_args.args[0])
        self.assertTrue(run.call_args.kwargs['check'])

    def test_unqualified_paid_feedback_run_is_blocked(self):
        with tempfile.TemporaryDirectory() as tmp:
            manifest = pathlib.Path(tmp, 'manifest.json')
            manifest.write_text(json.dumps({'repos': {}, 'cases': []}))
            args = SimpleNamespace(manifest=manifest, targets=1, agents=['claude:haiku'],
                                   agent_turn2=False, yes_spend=True)
            with patch.object(feedback_loop.subprocess, 'run') as run:
                with self.assertRaisesRegex(SystemExit, 'não foi qualificado'):
                    feedback_loop.run(args)
                run.assert_not_called()


class PairingTests(unittest.TestCase):
    @staticmethod
    def row(case, rep, arm, wall, tokens=None):
        return {'agent': 'codex', 'model': 'm', 'case': case, 'rep': rep, 'arm': arm,
                'category': 'direct', 'success': arm == 'ON', 'hit_at_1': True, 'wall_s': wall,
                'tokens_in': tokens, 'tokens_out': None, 'cost_usd': None, 'tool_calls': None,
                'malformed': False, 'edited_files': False}

    def test_rejects_duplicate_and_counts_unpaired(self):
        off = self.row('a', 0, 'OFF', 1)
        on = self.row('a', 0, 'ON', 2)
        pairs, incomplete = runner.paired_runs([off, on, self.row('b', 0, 'OFF', 1)])
        self.assertEqual((len(pairs), incomplete), (1, 1))
        with self.assertRaisesRegex(ValueError, 'duplicada'):
            runner.paired_runs([off, off, on])
        on['category'] = 'negative'
        with self.assertRaisesRegex(ValueError, 'metadados inconsistentes'):
            runner.paired_runs([off, on])

    def test_repetitions_are_averaged_per_task_and_missing_cost_is_not_zero(self):
        rows = [self.row('a', 0, 'OFF', 0), self.row('a', 0, 'ON', 2),
                self.row('a', 1, 'OFF', 0), self.row('a', 1, 'ON', 4),
                self.row('b', 0, 'OFF', 0), self.row('b', 0, 'ON', 9),
                self.row('c', 0, 'OFF', 0)]
        with tempfile.TemporaryDirectory() as tmp:
            source = pathlib.Path(tmp, 'runs.jsonl')
            output = pathlib.Path(tmp, 'report.md')
            source.write_text(''.join(json.dumps(row) + '\n' for row in rows))
            runner.report(source, output)
            text = output.read_text()
            self.assertIn('| codex / m | 3 | 2 | 1 | +6.00 (n=2 tarefas) (poucas tarefas) | n/d | n/d |', text)

    def test_empty_context_is_reported_separately_from_missing_data(self):
        empty = self.row('a', 0, 'ON', 1)
        empty['injected_chars'] = 0
        missing = self.row('b', 0, 'ON', 1)
        with tempfile.TemporaryDirectory() as tmp:
            source = pathlib.Path(tmp, 'runs.jsonl')
            output = pathlib.Path(tmp, 'report.md')
            source.write_text(''.join(json.dumps(row) + '\n' for row in [empty, missing]))
            runner.report(source, output)
            text = output.read_text()
            self.assertIn('| codex / m | 2 | 1/1 | n/a (prefixo) |', text)
            self.assertIn('nenhum prefixo foi entregue', text)


class FeedbackParsingTests(unittest.TestCase):
    def test_rejects_missing_or_invalid_fields(self):
        for value in ('{"files": [1], "helpful": [], "misleading": []}',
                      '{"files": [], "helpful": "E1", "misleading": []}',
                      '{"files": [], "helpful": [], "misleading": [1]}',
                      '{"files": []}'):
            with self.subTest(value=value):
                self.assertIsNone(feedback_loop.parse_report(value))
        self.assertEqual(feedback_loop.parse_report('{"files": [], "helpful": [], "misleading": []}'),
                         ([], [], []))


class FeedbackReportTests(unittest.TestCase):
    def test_failed_record_is_not_counted_as_saved_or_effect(self):
        row = {'agent': 'claude', 'model': 'm', 'malformed': False, 'feedback': {'helpful': ['x']},
               'record_ok': False, 'deterministic': None, 'evidence_noise': None, 'table_chars': 4,
               'probes': [{'rank_before': 2, 'rank_after': 1}]}
        with tempfile.TemporaryDirectory() as tmp:
            source = pathlib.Path(tmp, 'runs.jsonl')
            output = pathlib.Path(tmp, 'report.md')
            source.write_text(json.dumps(row) + '\n')
            feedback_loop.report(source, output)
            text = output.read_text()
            self.assertIn('| claude / m | 1 | 1/1 | 0/1 | 0/0 |', text)
            self.assertIn('| claude / m | 0 | n/d |', text)


class CorpusTests(unittest.TestCase):
    def test_rejects_dirty_and_wrong_checkout(self):
        with tempfile.TemporaryDirectory() as tmp:
            def git(*args):
                return subprocess.run(['git', '-C', tmp, *args], check=True, capture_output=True, text=True,
                                      env=dict(os.environ, GIT_AUTHOR_NAME='Test', GIT_AUTHOR_EMAIL='test@example.invalid',
                                               GIT_COMMITTER_NAME='Test', GIT_COMMITTER_EMAIL='test@example.invalid')).stdout.strip()
            git('init', '-q')
            pathlib.Path(tmp, 'file.txt').write_text('first')
            git('add', 'file.txt')
            git('commit', '-qm', 'first')
            first = git('rev-parse', 'HEAD')
            repos = {'repo': {'path': tmp, 'commit': first}}
            cases = [{'repo': 'repo'}]
            runner.validate_repos(repos, cases)
            pathlib.Path(tmp, 'file.txt').write_text('changed')
            with self.assertRaisesRegex(ValueError, 'alterações'):
                runner.validate_repos(repos, cases)
            git('add', 'file.txt')
            git('commit', '-qm', 'second')
            with self.assertRaisesRegex(ValueError, 'HEAD difere'):
                runner.validate_repos(repos, cases)

    def test_hook_failure_is_not_abstention(self):
        with patch.object(runner.subprocess, 'run', return_value=SimpleNamespace(stdout='', returncode=1)):
            with self.assertRaisesRegex(RuntimeError, 'hook falhou'):
                runner.sidecar_context('sidecar', 'home', 'repo', 'prompt')
        with patch.object(runner.subprocess, 'run', return_value=SimpleNamespace(stdout='invalid', returncode=0)):
            with self.assertRaisesRegex(RuntimeError, 'JSON inválido'):
                runner.sidecar_context('sidecar', 'home', 'repo', 'prompt')
        for malformed in ('{}', '{"hookSpecificOutput": {"additionalContext": "text"}}'):
            with self.subTest(malformed=malformed):
                with patch.object(runner.subprocess, 'run', return_value=SimpleNamespace(stdout=malformed, returncode=0)):
                    with self.assertRaisesRegex(RuntimeError, 'JSON inválido'):
                        runner.sidecar_context('sidecar', 'home', 'repo', 'prompt')

    def test_explicit_engine_overrides_process_environment(self):
        with patch.object(runner.subprocess, 'run', return_value=SimpleNamespace(stdout='', returncode=0)) as run:
            runner.sidecar_context('sidecar', 'home', 'repo', 'prompt', engine='v2')
        self.assertEqual(run.call_args.kwargs['env']['SIDECAR_ENGINE'], 'v2')
        with self.assertRaisesRegex(ValueError, 'engine'):
            runner.sidecar_context('sidecar', 'home', 'repo', 'prompt', engine='invalid')


class IsolationTests(unittest.TestCase):
    def test_real_hooks_rejected_before_touching_checkout(self):
        with tempfile.TemporaryDirectory() as repo:
            sentinel = pathlib.Path(repo, 'important')
            sentinel.write_text('keep')
            args = SimpleNamespace(real_hooks=True)
            with patch.object(runner.subprocess, 'run') as run:
                with self.assertRaises(ValueError):
                    runner.run_one(args, {'repo': {'path': repo}}, {'repo': 'repo'}, 'claude', 'm', 'ON', 0, 'home')
                run.assert_not_called()
            self.assertEqual(sentinel.read_text(), 'keep')

    def test_context_uses_fresh_session_each_run(self):
        result = SimpleNamespace(stdout='', returncode=0)
        with patch.object(runner.subprocess, 'run', return_value=result) as run:
            runner.sidecar_context('sidecar', 'home', 'repo', 'prompt')
            runner.sidecar_context('sidecar', 'home', 'repo', 'prompt')
        sessions = [json.loads(call.kwargs['input'])['session_id'] for call in run.call_args_list]
        self.assertNotEqual(sessions[0], sessions[1])

    def test_prefix_wall_time_includes_hook_cost(self):
        case = {'repo': 'repo', 'id': 'x', 'category': 'direct', 'prompt': 'p',
                'relevant': [{'path': 'a.go'}]}
        with tempfile.TemporaryDirectory() as out:
            args = SimpleNamespace(real_hooks=False, out_dir=out, timeout=1, max_budget=1, sidecar='sidecar', engine='v2')
            result = SimpleNamespace(stdout='{"type":"result","result":"{\\"files\\":[\\"a.go\\"]}"}', stderr='', returncode=0)
            status = SimpleNamespace(stdout='')
            with patch.object(runner, 'checkout'), patch.object(runner, 'sidecar_context', return_value=('context', 500)) as context, \
                 patch.object(runner.time, 'perf_counter', side_effect=[10.0, 12.0]), \
                 patch.object(runner.subprocess, 'run', side_effect=[result, status]):
                rec = runner.run_one(args, {'repo': {'path': out}}, case, 'claude', 'm', 'ON', 0, out)
            self.assertEqual(rec['wall_s'], 2.5)
            self.assertEqual(rec['sidecar_ms'], 500)
            self.assertEqual(rec['engine'], 'v2')
            self.assertEqual(context.call_args.kwargs['engine'], 'v2')

    def test_raw_output_is_opt_in_and_private(self):
        case = {'repo': 'repo', 'id': 'x', 'category': 'direct', 'prompt': 'p',
                'relevant': [{'path': 'a.go'}]}
        with tempfile.TemporaryDirectory() as out:
            args = SimpleNamespace(real_hooks=False, out_dir=out, timeout=1, max_budget=1, save_raw=True, engine='v2')
            result = SimpleNamespace(stdout='{}', stderr='', returncode=0)
            status = SimpleNamespace(stdout='')
            with patch.object(runner, 'checkout'), patch.object(runner.subprocess, 'run', side_effect=[result, status]):
                runner.run_one(args, {'repo': {'path': out}}, case, 'claude', 'm', 'OFF', 0, out)
            raw = pathlib.Path(out, 'raw', 'claude-OFF-x-0.txt')
            self.assertEqual(raw.stat().st_mode & 0o077, 0)
            self.assertEqual(raw.parent.stat().st_mode & 0o077, 0)

    def test_failed_or_dirty_run_never_counts_as_success(self):
        case = {'repo': 'repo', 'id': 'x', 'category': 'direct', 'prompt': 'p',
                'relevant': [{'path': 'a.go'}]}
        for code, dirty in ((1, ''), (0, ' M a.go')):
            with self.subTest(code=code, dirty=dirty), tempfile.TemporaryDirectory() as out:
                args = SimpleNamespace(real_hooks=False, out_dir=out, timeout=1, max_budget=1, engine='v2')
                result = SimpleNamespace(stdout='{"type":"result","result":"{\\"files\\":[\\"a.go\\"]}"}', stderr='', returncode=code)
                status = SimpleNamespace(stdout=dirty)
                with patch.object(runner, 'checkout'), patch.object(runner.subprocess, 'run', side_effect=[result, status]):
                    rec = runner.run_one(args, {'repo': {'path': out}}, case, 'claude', 'm', 'OFF', 0, out)
                self.assertFalse(rec['success'])


if __name__ == '__main__':
    unittest.main()


class ModelIdentityTests(unittest.TestCase):
    def test_served_model_is_read_from_model_usage(self):
        out = json.dumps({"type": "result", "result": '{"files": []}', "total_cost_usd": 0.01,
                          "modelUsage": {"claude-haiku-5-5": {"inputTokens": 2, "outputTokens": 66}}})
        self.assertEqual(runner.parse_output("claude", out)["served_models"], ["claude-haiku-5-5"])

    def test_claude_alias_is_rejected_and_explicit_id_accepted(self):
        with self.assertRaises(ValueError):
            runner.validate_model("claude", "haiku")
        runner.validate_model("claude", "claude-haiku-5-5")
        runner.validate_model("codex", "gpt-6-luna")  # other clients keep their own names

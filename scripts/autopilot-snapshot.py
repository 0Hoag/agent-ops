#!/usr/bin/env python3
"""Build bounded coordinator context; archive only completed, inactive pass artifacts."""
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import time


def query(repo, kind, fields):
    args = ['gh', kind, 'list', '-R', repo, '--state', 'open', '--limit', '10000', '--json', fields]
    value = json.loads(subprocess.check_output(args, text=True))
    if not isinstance(value, list):
        raise ValueError(f'{repo}: invalid {kind} inventory')
    return value


def report(path):
    text = path.read_text()
    first = text.splitlines()[0] if text.splitlines() else ''
    return text if first == 'AUTOPILOT: CONTINUE' or first.startswith('AUTOPILOT: STOP ') and first[16:].strip() else None


def main():
    directory = Path(sys.argv[1])
    lines = ['# Pass snapshot']
    for repo in sys.argv[2:]:
        lines.append(f'\n## {repo}')
        for issue in query(repo, 'issue', 'number,title,labels'):
            labels = [x['name'] for x in issue['labels'] if x['name'].startswith(('status:', 'wave:'))]
            lines.append(json.dumps({'issue': issue['number'], 'title': issue['title'], 'labels': labels}))
        for pr in query(repo, 'pr', 'number,baseRefName,headRefOid,statusCheckRollup'):
            checks = pr['statusCheckRollup']
            if not isinstance(checks, list):
                raise ValueError('invalid PR checks')
            lines.append(json.dumps({'pr': pr['number'], 'base': pr['baseRefName'], 'head': pr['headRefOid'],
                                     'checks': [c.get('conclusion') or c.get('status') or c.get('state') or 'UNKNOWN' for c in checks]}))
    workers = subprocess.run(['pgrep', '-fl', 'scripts/run-worker'], text=True, capture_output=True)
    if workers.returncode not in (0, 1):
        raise RuntimeError('worker inventory failed')
    lines.extend(['\n## Live workers', workers.stdout.strip() or '(none observed)'])
    # Only timestamp-named artifacts with a valid final report are completed passes.
    # A missing/malformed report means a potentially active/interrupted transcript: retain it.
    archive = directory / 'archive'
    archive.mkdir(exist_ok=True)
    processes = subprocess.check_output(['ps', '-ax', '-o', 'command='], text=True)
    reports = []
    for parent in (directory, archive):
        for path in parent.glob('*.md'):
            if not re.fullmatch(r'\d{8}-\d{6}', path.stem):
                continue
            final = report(path)
            if final:
                reports.append((path.stem, final))
                if parent == directory and path.stem not in processes:
                    files = [directory / (path.stem + ext) for ext in ('.md', '.jsonl', '.err')]
                    existing = [p for p in files if p.exists()]
                    if all(time.time() - p.stat().st_mtime > 86400 for p in existing):
                        for source in existing:
                            target = archive / source.name
                            if target.exists():
                                raise ValueError(f'archive collision: {target}')
                            source.rename(target)
    lines.append('\n## Last three final pass reports')
    for stamp, final in sorted(reports, reverse=True)[:3]:
        lines.extend([f'### {stamp}', '\n'.join(final.splitlines()[:10])])
    temporary = directory / 'snapshot.md.tmp'
    temporary.write_text('\n'.join(lines) + '\n')
    os.replace(temporary, directory / 'snapshot.md')


if __name__ == '__main__':
    try:
        main()
    except (OSError, ValueError, KeyError, TypeError, subprocess.SubprocessError) as error:
        print(f'API ERROR: pass snapshot unavailable: {error}', file=sys.stderr)
        sys.exit(2)

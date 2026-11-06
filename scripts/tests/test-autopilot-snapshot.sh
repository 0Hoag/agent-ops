#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
python3 - "$root" <<'PY'
import json, os, pathlib, shutil, subprocess as sp, sys, tempfile, time
source=pathlib.Path(sys.argv[1])
with tempfile.TemporaryDirectory() as temp:
 root=pathlib.Path(temp); repo=root/'repo'; scripts=repo/'scripts'; scripts.mkdir(parents=True)
 for name in ('autopilot.sh','autopilot-state.sh','autopilot-snapshot.py'): shutil.copy2(source/'scripts'/name,scripts/name)
 sp.run(['git','init','-q',str(repo)],check=True)
 bin=root/'bin'; bin.mkdir()
 def exe(path,text): path.write_text(text); path.chmod(0o755)
 for name in ('worktrees.sh','branches.sh','audit.sh'): exe(scripts/name,'#!/bin/sh\nexit 0\n')
 exe(bin/'pgrep','#!/bin/sh\ncase "$*" in *-fl*) echo "123 scripts/run-worker.sh W17p fixture";; *) exit 1;; esac\n')
 exe(bin/'ps','#!/bin/sh\necho "opencode 20200101-000004.jsonl"\n')
 exe(bin/'gh', '''#!/usr/bin/env python3
import json,os,sys
a=sys.argv
if '--jq' in a: sys.exit(0)
if os.environ.get('FAIL')=='api': sys.exit(1)
if os.environ.get('FAIL')=='malformed': print('{}'); sys.exit(0)
if 'issue' in a: print(json.dumps([{'number':430,'title':'snapshot fixture','labels':[{'name':'status:todo'},{'name':'wave:W17p'},{'name':'area:docs'}]}]))
else: print(json.dumps([{'number':431,'baseRefName':'reviews/development','headRefOid':'a'*40,'statusCheckRollup':[{'conclusion':'SUCCESS'},{'status':'IN_PROGRESS'}]}]))
''')
 exe(bin/'opencode', '''#!/usr/bin/env python3
import json,pathlib,sys
p=pathlib.Path('.agent-runs/coordinator/snapshot.md')
snapshot=p.read_text()
assert sys.argv[-1].endswith(snapshot), 'exact snapshot missing from prompt'
assert 'Do not load the orchestrator-workers skill' in sys.argv[-1]
assert 'Do not list .agent-runs/coordinator/' in sys.argv[-1]
pathlib.Path('prompt.txt').write_text(sys.argv[-1])
print(json.dumps({'type':'text','part':{'text':'AUTOPILOT: STOP fixture finished'}}))
''')
 env=dict(os.environ,PATH=str(bin)+os.pathsep+os.environ['PATH'],REPOS='fixture/api fixture/client')
 env.pop('AUTOPILOT_REQUIRED_TOOLS',None); env.pop('AUTOPILOT_PREFLIGHT_PROBE',None)
 directory=repo/'.agent-runs/coordinator'
 def fixture():
  if directory.exists(): shutil.rmtree(directory)
  directory.mkdir(parents=True)
  for n in range(1,6):
   for ext in ('md','jsonl','err'):
    p=directory/f'20200101-00000{n}.{ext}'
    p.write_text('AUTOPILOT: CONTINUE\nreport '+str(n) if ext=='md' and n!=3 else 'active or raw transcript')
    if n!=5: os.utime(p,(time.time()-90000,)*2)
  for name in ('snapshot.md','audit.txt','helper.sh'): (directory/name).write_text('preserve')
 def run(): return sp.run(['bash','scripts/autopilot.sh'],cwd=repo,env=env,text=True,capture_output=True,timeout=10)
 def verify():
  p=run(); assert p.returncode==0, p.stdout+p.stderr
  snapshot=(directory/'snapshot.md').read_text()
  assert 'fixture/api' in snapshot and 'fixture/client' in snapshot, 'repository inventory missing'
  assert snapshot.count('"issue": 430')==2 and 'status:todo' in snapshot and 'wave:W17p' in snapshot
  assert 'area:docs' not in snapshot and '"base": "reviews/development"' in snapshot and 'a'*40 in snapshot
  assert 'SUCCESS' in snapshot and 'IN_PROGRESS' in snapshot and '123 scripts/run-worker.sh' in snapshot
  assert '### 20200101-000005' in snapshot and '### 20200101-000004' in snapshot and '### 20200101-000002' in snapshot
  assert '### 20200101-000001' not in snapshot and 'active or raw transcript' not in snapshot
  for n in (1,2): assert (directory/'archive'/f'20200101-00000{n}.jsonl').exists(), 'old completed pass not archived'
  for n in (3,4,5): assert (directory/f'20200101-00000{n}.jsonl').exists(), 'active/recent transcript archived'
  for name in ('snapshot.md','audit.txt','helper.sh'): assert (directory/name).exists()
  assert 'coordinator asked to stop' in p.stdout, 'valid final report rejected'
 fixture(); verify(); print('PASS inventory, exact prompt, age/archive exclusions, final STOP report')
 original=(scripts/'autopilot.sh').read_text()
 line='  python3 scripts/autopilot-snapshot.py "$dir" $REPOS || exit 2\n'
 assert original.count(line)==1
 (scripts/'autopilot.sh').write_text(original.replace(line,''))
 fixture()
 try: verify()
 except AssertionError as e:
  assert 'exact snapshot missing' in str(e) or 'repository inventory missing' in str(e), str(e)
  print('CONTROL PASS: identical fixture detects removed production snapshot/archive call:',e)
 else: raise AssertionError('removed production change survived')
 (scripts/'autopilot.sh').write_text(original)
 fixture(); verify(); print('RESTORED PASS identical fixture')
 for failure in ('api','malformed'):
  fixture(); env['FAIL']=failure
  p=run(); assert p.returncode==2 and 'API ERROR: pass snapshot unavailable' in p.stderr, p.stdout+p.stderr
  assert (directory/'snapshot.md').read_text()=='preserve', 'failed inventory replaced snapshot'
  print('PASS fail-safe snapshot',failure)
PY

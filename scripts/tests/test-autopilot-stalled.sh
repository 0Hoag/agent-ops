#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
python3 - "$root" <<'PY'
import concurrent.futures
import json
import os
from pathlib import Path
import shutil
import subprocess as sp
import sys
import tempfile

source = Path(sys.argv[1])
def check(ok, message):
    if not ok:
        raise AssertionError(message)

with tempfile.TemporaryDirectory(prefix='autopilot-test-') as temp:
    tmp = Path(temp)
    repo = tmp / 'repo'
    scripts = repo / 'scripts'
    scripts.mkdir(parents=True)
    for name in ('autopilot.sh', 'autopilot-state.sh', 'spawn.sh'):
        shutil.copy2(source / 'scripts' / name, scripts / name)
    sp.run(['git', 'init', '-q', str(repo)], check=True)
    bin = tmp / 'bin'
    bin.mkdir()
    def executable(path, text):
        path.write_text(text)
        path.chmod(0o755)
    for name in ('worktrees.sh', 'branches.sh', 'audit.sh'):
        executable(scripts / name, '#!/bin/sh\nexit 0\n')
    executable(bin / 'pgrep', '#!/bin/sh\n[ "$CASE" = worker ] && echo "$LIVE_PID"\n')
    executable(bin / 'sleep', '#!/bin/sh\necho "$1" >> "$SLEEPS"\n[ "$1" = 2 ] && touch "$WAKE"\n')
    executable(bin / 'opencode', '''#!/usr/bin/env python3
import json, os, pathlib, sys
p=pathlib.Path(os.environ['PASSES'])
n=int(p.read_text())+1 if p.exists() else 1
p.write_text(str(n))
c=os.environ['CASE']
if n>1:
 print(json.dumps({'type':'text','part':{'text':'AUTOPILOT: STOP fixture finished'}}))
elif c=='quota':
 print('usage limit', file=sys.stderr)
elif c=='empty': pass
elif c in ('invalid-json','invalid-quota'):
 print('{broken')
 if c=='invalid-quota': print('usage limit',file=sys.stderr)
else:
 text='bad report' if c=='malformed' else 'AUTOPILOT: CONTINUE'
 print(json.dumps({'type':'text','part':{'text':text}}))
''')
    # Emulate gh's field selection AND --jq with the real jq executable.
    executable(bin / 'gh', '''#!/usr/bin/env python3
import json, os, pathlib, subprocess, sys
a=sys.argv[1:]; c=os.environ['CASE']
if a[:2]==['pr','merge']:
 sys.exit('unexpected merge')
if c=='api-error' and a[0]=='api': sys.exit(1)
if a[0]=='api': data={'sha':'a'*40}
elif a[:2]==['pr','list']:
 checks=[{'status':'IN_PROGRESS','conclusion':None}] if c=='pr-ci' else []
 data=[{'number':9,'updatedAt':'2026-11-05T00:00:00Z','headRefOid':'b'*40,'statusCheckRollup':checks,'isDraft':True,'labels':[],'body':'','baseRefName':'reviews/development','headRefName':'fixture'}] if checks else []
elif a[:2]==['run','list']:
 data=[{'status':'queued','conclusion':None}] if c=='dev-ci' else []
elif a[:2]==['issue','list']:
 if '--label' in a: data=[]
 else:
  data=[{'number':408,'labels':[{'name':'status:blocked'}],'title':'fixture','body':'blocked','updatedAt':'2026-11-05T00:00:00Z','comments':[]}]
  fields=a[a.index('--json')+1]
  if fields=='number,labels,title,body,updatedAt':
   p=pathlib.Path(os.environ['QUERIES']); n=int(p.read_text())+1 if p.exists() else 1; p.write_text(str(n))
  n=int(pathlib.Path(os.environ['QUERIES']).read_text()) if pathlib.Path(os.environ['QUERIES']).exists() else 0
  if n>1:
   if c=='blocked' and pathlib.Path(os.environ['WAKE']).exists():
    data[0]['updatedAt']='2026-11-05T01:00:00Z'
    pathlib.Path(os.environ['WAKE']).unlink()
   if c=='labels': data[0]['labels'].append({'name':'wake'})
   if c=='body': data[0]['body']='unblocked'
   if c=='comments': data[0]['comments']=[{'id':'1','body':'authorized','updatedAt':'2026-11-05T01:00:00Z'}]
else: sys.exit('unexpected gh '+repr(a))
if '--json' in a:
 fields=a[a.index('--json')+1].split(','); data=[{k:v for k,v in row.items() if k in fields} for row in data]
if '--jq' in a:
 p=subprocess.run([os.environ['REAL_JQ'],'-r',a[a.index('--jq')+1]],input=json.dumps(data),text=True)
 sys.exit(p.returncode)
print(json.dumps(data))
''')
    env = dict(os.environ, PATH=str(bin)+os.pathsep+os.environ['PATH'], REAL_JQ=shutil.which('jq'),
               REPOS='fixture/repo', RUN_DIRS=str(tmp/'none'), INTERVAL='1', POLL='1',
               MIN_GAP='0', MERGE_EVERY='100', STALL_SLEEP='2', LIVE_PID=str(os.getpid()))
    env.pop('AUTOPILOT_REQUIRED_TOOLS', None)
    env.pop('AUTOPILOT_PREFLIGHT_PROBE', None)
    def run_case(case, expected, diagnostic):
        for name in ('passes','queries','sleeps','notifications','wake'):
            path=tmp/name
            if path.exists(): path.unlink()
        e=dict(env, CASE=case, PASSES=str(tmp/'passes'), QUERIES=str(tmp/'queries'), SLEEPS=str(tmp/'sleeps'),
               NOTIFICATIONS=str(tmp/'notifications'), WAKE=str(tmp/'wake'), NOTIFY_CMD='echo stalled >> "$NOTIFICATIONS"')
        try:
            p=sp.run(['bash',str(scripts/'autopilot.sh')],cwd=repo,env=e,text=True,stdout=sp.PIPE,stderr=sp.STDOUT,timeout=5)
        except sp.TimeoutExpired:
            raise AssertionError(case+': bounded STALLED guard failed: infinite idle wait')
        print('CASE',case,'exit',p.returncode,'\n'+p.stdout)
        check(p.returncode==expected and diagnostic in p.stdout, case+': intended behavior missing')
        if case=='quota': check((tmp/'sleeps').read_text().splitlines()==['900'], 'quota: exact 15-minute retry')
        if case=='blocked':
            check((tmp/'sleeps').read_text().splitlines()==['1','2'], 'blocked: one poll sleep and one stall sleep before fingerprint wake')
            check((tmp/'notifications').read_text().splitlines()==['stalled'], 'blocked: one notification per stalled period')
        if case in ('worker','pr-ci','dev-ci','labels','body','comments'):
            check((tmp/'passes').read_text()=='2',case+': must wake one new pass')
    run_case('blocked',0,'GitHub changed')
    for case in ('worker','pr-ci','dev-ci'):
        run_case(case,0,'bounded heartbeat')
    for case in ('labels','body','comments'):
        run_case(case,0,'GitHub changed')
    run_case('api-error',2,'API ERROR:')
    for case in ('empty','malformed','invalid-json','invalid-quota'):
        run_case(case,2,'invalid final output')
    run_case('quota',0,'usage limit, waiting 15 min')

    # Identical blocked fixture, only the actual production return guard is removed.
    original=(scripts/'autopilot.sh').read_text()
    guard='    [ "$now" != "$base" ] && { echo "  GitHub changed, next pass now"; return; }\n  done\n}'
    check(original.count(guard)==1,'stall mutation must change exactly one production guard')
    (scripts/'autopilot.sh').write_text(original.replace(guard,'    waited=0\n  done\n}'))
    try:
        run_case('blocked',2,'STALLED:')
    except AssertionError as e:
        check('infinite idle wait' in str(e),'stall control must fail by bounded infinite-wait assertion')
        print('CONTROL PASS:',e)
    else: raise AssertionError('stall mutation survived identical fixture')
    (scripts/'autopilot.sh').write_text(original)

    # A no-op mutation must not be mistaken for a failing production guard.
    try:
        run_case('blocked',2,'STALLED:')
    except AssertionError as e:
        check('intended behavior missing' in str(e),'no-op control must reject the false control')
        print('NO-OP CONTROL PASS:',e)
    else: raise AssertionError('no-op mutation falsely survived control')

    sentinel=tmp/'children'
    executable(bin/'child', '#!/bin/sh\necho child >> "$SENTINEL"\n')
    spawn_env=dict(env,SENTINEL=str(sentinel))
    harness_bin=tmp/'harness-bin'; harness_bin.mkdir()
    for name in ('git','gh','jq'):
        (harness_bin/name).symlink_to(shutil.which(name))
    executable(harness_bin/'codex','#!/bin/sh\nexit 0\n')
    for harness,code in (('codex',0),('opencode',1)):
        p=sp.run(['/bin/bash','-c','. "$1"; autopilot_preflight "$2"','_',str(scripts/'autopilot-state.sh'),harness],env=dict(spawn_env,PATH=str(harness_bin)),text=True,stdout=sp.PIPE,stderr=sp.STDOUT)
        check(p.returncode==code,'harness-specific '+harness+' preflight: '+p.stdout)
        if code: check('preflight failed: missing opencode' in p.stdout,'specific missing harness diagnostic')
    print('PASS Codex preflight without OpenCode; missing OpenCode rejected for its own harness')
    launched=[]
    def spawn(issue,unit='unit',launcher=str(bin/'child'),extra=None,cwd=None):
        p=sp.run(['bash',str(scripts/'spawn.sh'),'--recovery','fixture/repo',str(issue),unit,str(tmp/'child.log'),launcher], cwd=cwd or repo,env=dict(spawn_env,**(extra or {})),text=True,stdout=sp.PIPE,stderr=sp.STDOUT)
        print('SPAWN',issue,unit,'exit',p.returncode,'\n'+p.stdout)
        if p.returncode==0: launched.append((issue,unit))
        return p
    def refused(p, diagnostic):
        check(p.returncode!=0 and diagnostic in p.stdout, 'expected narrow rejection '+diagnostic+': '+p.stdout)
    def duplicate_case(issue):
        p=spawn(issue); check(p.returncode==0 and 'recovery reserved:' in p.stdout,'valid documented spawn: '+p.stdout)
        refused(spawn(issue), 'recovery refused: already reserved')
        refused(spawn(issue,'renamed',cwd=tmp), 'recovery refused: already reserved')
    refused(spawn(1,launcher='missing-launcher-fixture'),'preflight failed: missing launcher missing-launcher-fixture')
    check(not (repo/'.git/recovery').exists(),'missing launcher consumed reservation')
    executable(bin/'probe', '#!/bin/sh\necho browser-unavailable >&2\nexit 1\n')
    refused(spawn(1,extra={'AUTOPILOT_PREFLIGHT_PROBE':str(bin/'probe')}),'preflight failed: environment probe')
    check(not (repo/'.git/recovery').exists(),'environment preflight consumed reservation')
    refused(spawn(1,extra={'AUTOPILOT_REQUIRED_TOOLS':'missing-browser-fixture'}),'preflight failed: missing missing-browser-fixture')
    check(not (repo/'.git/recovery').exists(),'required tool preflight consumed reservation')
    duplicate_case(1)
    # Let actual detached child finish, without relying on a fake sleep.
    import time
    deadline=time.monotonic()+3
    while not sentinel.exists() and time.monotonic()<deadline: time.sleep(.01)
    check(sentinel.read_text().splitlines()==['child'],'valid spawn executes child exactly once')
    with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
        results=list(pool.map(lambda u:spawn(2,u), ['one','renamed']))
    check(sorted(p.returncode for p in results)==[0,1],'atomic same-issue duplicate refusal')
    refused(next(p for p in results if p.returncode), 'recovery refused: already reserved')
    # A real linked worktree must share the same canonical git directory.
    sp.run(['git','-C',str(repo),'-c','user.name=Fixture','-c','user.email=fixture@example.com','add','scripts'],check=True)
    sp.run(['git','-C',str(repo),'-c','user.name=Fixture','-c','user.email=fixture@example.com','commit','-qm','fixture'],check=True)
    linked=tmp/'linked'
    sp.run(['git','-C',str(repo),'worktree','add','-qb','linked',str(linked)],check=True)
    p=sp.run(['bash',str(linked/'scripts/spawn.sh'),'--recovery','fixture/repo','1','renamed',str(tmp/'linked.log'),str(bin/'child')],cwd=tmp,env=spawn_env,text=True,stdout=sp.PIPE,stderr=sp.STDOUT)
    refused(p,'recovery refused: already reserved')
    deadline=time.monotonic()+3
    while len(sentinel.read_text().splitlines())<2 and time.monotonic()<deadline: time.sleep(.01)
    check(sentinel.read_text().splitlines()==['child','child'],'duplicate and renamed dispatch must not execute child')
    legacy=sp.run(['bash',str(scripts/'spawn.sh'),str(tmp/'legacy.log'),'/usr/bin/true'],cwd=tmp,env=spawn_env,text=True,stdout=sp.PIPE,stderr=sp.STDOUT)
    check(legacy.returncode==0,'legacy arbitrary command compatible')
    # Mutate the atomic publication itself, not a cosmetic diagnostic or inputs.
    helper=scripts/'autopilot-state.sh'; original=helper.read_text()
    def baseline(issue):
        before=set((repo/'.git/recovery').iterdir())
        duplicate_case(issue)
        # Reset only the fixture reservation so the control receives identical inputs.
        for path in set((repo/'.git/recovery').iterdir())-before: path.unlink()
    baseline(3)
    guard='if ! ln "$tmp" "$file" 2>/dev/null; then rm -f "$tmp"; return 1; fi'
    check(original.count(guard)==1,'atomic mutation must change exactly one real guard')
    helper.write_text(original.replace(guard,'mv -f "$tmp" "$file"'))
    try:
        duplicate_case(3)
    except AssertionError as e:
        check('expected narrow rejection recovery refused:' in str(e),'atomic control wrong failure')
        print('CONTROL PASS: duplicate accepted after atomic publication guard mutation')
    else: raise AssertionError('atomic mutation survived identical fixture')
    helper.write_text(original)
    baseline(4)
    key='file="$state_root/$(printf \'%s|%s\' "$repo" "$issue" | shasum | awk \'{print $1}\')"'
    check(original.count(key)==1,'identity mutation must change exactly one production guard')
    helper.write_text(original.replace(key,'file="$state_root/$(printf \'%s\' "$identity" | shasum | awk \'{print $1}\')"'))
    try:
        duplicate_case(4)
    except AssertionError as e:
        check('expected narrow rejection recovery refused:' in str(e),'identity control wrong failure')
        print('CONTROL PASS: renamed identity accepted after stable key guard mutation')
    else: raise AssertionError('identity mutation survived identical fixture')
    helper.write_text(original)
    deadline=time.monotonic()+3
    while len(sentinel.read_text().splitlines())<len(launched) and time.monotonic()<deadline: time.sleep(.01)
    check(len(sentinel.read_text().splitlines())==len(launched),'all detached fixture children finished before cleanup')
    print('PASS real-jq production-loop, spawn, atomic/cross-worktree and guard controls')
PY

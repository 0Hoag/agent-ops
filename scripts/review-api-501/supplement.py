from http_common import *
import uuid
s=json.loads((OUT/'state.json').read_text());a=s['a']
identity='review/duplicate-'+uuid.uuid4().hex
for attempt in range(2):call('POST','/api/v1/repositories',{'provider':'gitlab','full_name':identity},a,case='duplicate-'+str(attempt))
call('POST','/api/v1/agent-runs/00000000-0000-0000-0000-000000000001/timeline',{'steps':[]},extra={'X-Agent-Run-Token':'synthetic-ingestion-501'},case='missing-run')
call('GET','/api/v1/agent-runs/aggregate?repository_id=invalid',token=a,case='invalid-uuid')
body={'project':{'path_with_namespace':'review/synthetic'},'object_attributes':{'id':3,'iid':3,'title':'Webhook state control','state':'opened','updated_at':'2026-11-12T00:00:00Z'}}
for state in ('opened','open'):
 body['object_attributes']['state']=state
 call('POST','/api/v1/webhooks/gitlab',body,extra={'X-Gitlab-Token':'review-secret','X-Gitlab-Event':'Issue Hook','X-Gitlab-Event-UUID':str(uuid.uuid4())},case='webhook-'+state)
call('POST',f"/api/v1/agent-runs/{s['runid']}/timeline",{'steps':[{'input_tokens':11}]},extra={'X-Agent-Run-Token':'synthetic-ingestion-501'},case='seed-before-clear')
assert len(call('GET',f"/api/v1/agent-runs/{s['runid']}/timeline",token=a,case='read-before-clear')['data'])==1
# The exact existing-run empty payload used by the first failing HTTP sweep.
call('POST',f"/api/v1/agent-runs/{s['runid']}/timeline",{},extra={'X-Agent-Run-Token':'synthetic-ingestion-501'},case='empty-timeline-after')
assert not call('GET',f"/api/v1/agent-runs/{s['runid']}/timeline",token=a,case='read-after-clear')['data']
(OUT/'supplement-results.json').write_text(json.dumps(rows,indent=2))
for x in rows:print(x['case'],x['status'],x['response'])

from http_common import *
login={'email':'review501@example.invalid','password':'synthetic-review-password'}
t=call('POST','/api/v1/auth/login',login)['token']
t2=call('POST','/api/v1/auth/login',{**login,'email':'other501@example.invalid'})['token']
t3=call('POST','/api/v1/auth/login',{**login,'email':'member501@example.invalid'})['token']
o=call('POST','/api/v1/organizations',{'name':'Review 501'},t)['data']['id']
u=call('POST','/api/v1/organizations',{'name':'Other 501'},t2)['data']['id']
a=call('POST','/api/v1/auth/select-organization',{'organization_id':o},t)['token']
b=call('POST','/api/v1/auth/select-organization',{'organization_id':u},t2)['token']
repo={'provider':'gitlab','full_name':'review/synthetic','gitlab_base_url':'http://host.docker.internal:35082','webhook_secret':'review-secret','backfill_token':'review-provider-token'}
r=call('POST','/api/v1/repositories',repo,a)['data']['id']
e=call('POST','/api/v1/repositories',{**repo,'full_name':'review/empty'},a)['data']['id']
w=call('POST','/api/v1/waves',{'name':'Review 501','brief':'Synthetic review','state':'todo'},a)['data']['id']
run={'repository_id':r,'wave':'review501','worker':'synthetic','model':'review-model','input_tokens':10,'output_tokens':5,'started_at':'2026-11-12T00:00:00Z','ended_at':'2026-11-12T00:01:00Z'}
runid=call('POST','/api/v1/agent-runs',run,extra={'X-Agent-Run-Token':'synthetic-ingestion-501'})['data']['id']
budget={'repository_id':r,'wave':'review501','usd_limit':10,'starts_at':'2026-11-01T00:00:00Z','ends_at':'2026-12-01T00:00:00Z'}
bid=call('POST','/api/v1/budgets',budget,a)['data']['id']
price={'model':'review-model','effective_date':'2026-11-01','input_per_million':1,'output_per_million':2}
pid=call('POST','/api/v1/agent-run-prices',price,a)['data']['id']
member=call('GET','/api/v1/auth/session',token=t3)['account_id']
worker={'Wave':'review501','Branch':'review/synthetic','Brief':'/review/worker.sh','Worktree':'/review','Script':'review'}
job=call('POST','/api/v1/workers/start',worker,a)['data']['ID']
state=dict(a=a,b=b,t=t,t2=t2,t3=t3,o=o,u=u,r=r,e=e,w=w,run=run,runid=runid,repo=repo,budget=budget,bid=bid,price=price,pid=pid,member=member,worker=worker,job=job,login=login)
(OUT/'state.json').write_text(json.dumps(state))
(OUT/'setup-results.json').write_text(json.dumps(rows,indent=2))
print('Setup complete')

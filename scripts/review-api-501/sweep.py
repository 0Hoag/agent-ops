from http_common import *
import socket,uuid
v=json.loads((OUT/'state.json').read_text());globals().update(v)
emptyorg=call('POST','/api/v1/organizations',{'name':'Empty isolated'},t)['data']['id']
emptytoken=call('POST','/api/v1/auth/select-organization',{'organization_id':emptyorg},t)['token']
job=call('POST','/api/v1/workers/start',worker,a)['data']['ID']
call('POST',f'/api/v1/repositories/{r}/sync',token=a)
s=json.load(open(ROOT/'api/docs/swagger.json'))
subs={'organization_id':o,'repository_id':r,'wave_id':w,'run_id':runid,'budget_id':bid,'price_id':pid,'account_id':member,'worker_id':job,'provider':'gitlab','kind':'slack','number':'1'}
webhook={'object_kind':'issue','project':{'path_with_namespace':'review/synthetic'},'object_attributes':{'id':2,'iid':2,'title':'Webhook fixture','state':'opened','action':'open','created_at':'2026-11-12T00:00:00Z','updated_at':'2026-11-12T00:00:00Z'},'labels':[]}
def sample(schema):
 if '$ref' in schema:return sample(s['definitions'][schema['$ref'].split('/')[-1]])
 typ=schema.get('type','object')
 if typ=='object':return {k:sample(v) for k,v in schema.get('properties',{}).items()}
 if typ=='array':return []
 if typ in ('integer','number'):return 1
 if typ=='boolean':return False
 return '2026-11-12T00:00:00Z' if schema.get('format')=='date-time' else 'review'
def ws(p,tok,case,path):
 h={'Host':'127.0.0.1:35081','Connection':'Upgrade','Upgrade':'websocket','Sec-WebSocket-Version':'13','Sec-WebSocket-Key':'dGhlIHNhbXBsZSBub25jZQ=='}
 if tok:h['Authorization']='Bearer '+tok
 with socket.create_connection(('127.0.0.1',35081),timeout=5) as sock:
  sock.sendall(('GET '+p+' HTTP/1.1\r\n'+''.join(k+': '+v+'\r\n' for k,v in h.items())+'\r\n').encode())
  raw=sock.recv(16384).decode(errors='replace');status=int(raw.split()[1])
 rows.append(dict(method='GET',route=path,path=p,case=case,status=status,shape='websocket upgrade' if status==101 else raw.split('\r\n\r\n')[-1],response=raw))
for path,ops in s['paths'].items():
 for method,op in ops.items():
  m=method.upper();body=next((sample(x['schema']) for x in op.get('parameters',[]) if x['in']=='body'),None)
  if path=='/api/v1/auth/login':body=login
  if path=='/api/v1/auth/select-organization':body={'organization_id':o}
  if path=='/api/v1/repositories' and m=='POST':body=repo
  if path=='/api/v1/repositories/{repository_id}' and m=='PATCH':body={'status_mappings':{}}
  if path=='/api/v1/repositories/test':body={'provider':'gitlab','full_name':'review/synthetic','base_url':'http://host.docker.internal:35082','token':'review-provider-token'}
  if path=='/api/v1/agent-runs' and m=='POST':body=run
  if path.endswith('/timeline') and m=='POST':body={'steps':[{'input_tokens':10,'output_tokens':5,'tool_name':'synthetic'}]}
  if 'agent-run-prices' in path and m in ('POST','PATCH'):body=price
  if path.startswith('/api/v1/budgets') and m in ('POST','PATCH'):body=budget
  if path.startswith('/api/v1/waves') and m in ('POST','PATCH'):body={'name':'matrix','brief':'Synthetic','state':'todo'}
  if path=='/api/v1/alerts/channels' and m=='POST' or path=='/api/v1/alerts/channels/{kind}' and m=='PUT':body={'kind':'slack','enabled':False,'slack_webhook_url_env_ref':'REVIEW_UNSET_SLACK'}
  if path=='/api/v1/alerts/thresholds' and m=='PUT':body={'min_cache_hit':0.8,'silent_seconds':300,'pr_stale_seconds':86400}
  if '/workers/' in path and m=='POST' and path.endswith(('start','restart')):body=worker
  if path.endswith('/invitations'):body={'role':'viewer'}
  if path.endswith('/members') and m=='POST':body={'account_id':member,'role':'viewer'}
  if path.endswith('/members/{account_id}') and m=='PATCH':body={'role':'viewer'}
  if '/webhooks/' in path:body=webhook
  for case,tok,payload in [('no-token',None,body),('wrong-organization',b,body),('missing-body',a,None),('invalid-body',a,b'{'),('empty-data',a,{} if body is not None else None),('valid',a,body)]:
   p=path
   for k,val in subs.items():p=p.replace('{'+k+'}',val)
   if '/work/' in path:p=p.replace('/slack/','/issue/')
   query={}
   if path in ('/api/v1/work','/api/v1/insights','/api/v1/board','/api/v1/board/ws','/api/v1/work/{kind}/{number}'):query['repository_id']=e if case=='empty-data' else r
   if path in ('/api/v1/overview','/api/v1/insights/analytics','/api/v1/insights/delivery'):query.update(repository_id=e if case=='empty-data' else r,**{'from':'2026-11-01','to':'2026-11-13'})
   if case=='invalid-body' and m=='GET':query.update(repository_id='invalid',page='invalid',**{'from':'invalid'})
   if query:p+='?'+urllib.parse.urlencode(query)
   ingest=(path=='/api/v1/agent-runs' or path.endswith('/timeline')) and m=='POST'
   extra={'X-Agent-Run-Token':'synthetic-ingestion-501'} if ingest and case not in ('no-token','wrong-organization') else {}
   if '/webhooks/' in path:
    extra={'X-Gitlab-Token':'wrong' if case=='wrong-organization' else 'review-secret','X-Gitlab-Event':'Issue Hook','X-Gitlab-Event-UUID':str(uuid.uuid4())} if case!='no-token' else {}
   # Use independent owned records for destructive calls and unique creates.
   if path=='/api/v1/repositories' and m=='POST' and payload is body:payload={**repo,'full_name':'review/'+uuid.uuid4().hex}
   if path=='/api/v1/agent-run-prices' and m=='POST' and payload is body:payload={**price,'model':'review-'+uuid.uuid4().hex}
   if m=='DELETE' and case not in ('no-token','wrong-organization'):
    if '/repositories/' in path:
     rid=call('POST','/api/v1/repositories',{**repo,'full_name':'review/'+uuid.uuid4().hex},a)['data']['id'];p=p.replace(r,rid)
    if '/waves/' in path:
     wid=call('POST','/api/v1/waves',{'name':'delete','brief':'Synthetic deletion fixture','state':'todo'},a)['data']['id'];p=p.replace(w,wid)
    if '/budgets/' in path:
     did=call('POST','/api/v1/budgets',budget,a)['data']['id'];p=p.replace(bid,did)
    if '/members/' in path:call('POST',f'/api/v1/organizations/{o}/members',{'account_id':member,'role':'viewer'},a)
   if path.endswith('/members') and m=='POST' and case=='valid':call('DELETE',f'/api/v1/organizations/{o}/members/{member}',token=a)
   if path.endswith('/members/{account_id}') and m=='PATCH':call('POST',f'/api/v1/organizations/{o}/members',{'account_id':member,'role':'viewer'},a)
   if path=='/api/v1/organization-invitations/accept':
    invitation=call('POST',f'/api/v1/organizations/{o}/invitations',{'role':'viewer'},a)['data']['token']
    if case not in ('missing-body','invalid-body','empty-data'):payload={'token':invitation if case!='wrong-organization' else 'invalid-invitation'}
    # A second account intentionally accepts the invitation, as supported by this account-scoped route.
    if case=='valid':
     call('DELETE',f'/api/v1/organizations/{o}/members/{member}',token=a)
     tok=t3
   if case=='empty-data' and m=='GET' and path in ('/api/v1/agent-runs','/api/v1/agent-runs/aggregate','/api/v1/agent-run-prices','/api/v1/agent-run-prices/missing','/api/v1/alerts/history','/api/v1/budgets','/api/v1/waves'):tok=emptytoken
   try:
    if path.endswith('/ws') or path.endswith('/logs'):ws(p,tok,case,path)
    else:call(m,p,payload,tok,extra,case,path)
   except Exception as exc:rows.append(dict(method=m,route=path,path=p,case=case,status='ERROR',shape=str(exc)))
(OUT/'final-http-results.json').write_text(json.dumps(rows,indent=2))
with (OUT/'final-matrix.md').open('w') as f:
 f.write('| Method / route | No token | Other org | Missing body | Invalid body/query | Empty | Valid |\n|---|---|---|---|---|---|---|\n')
 for path,ops in s['paths'].items():
  for method in ops:
   rr=[x for x in rows if x['route']==path and x['method']==method.upper() and x['case']!='setup']
   f.write('| '+method.upper()+' '+path+' | '+' | '.join(str(x['status'])+' '+x['shape'] for x in rr)+' |\n')
print('Recorded',len(rows),'requests;',sum(len(x) for x in s['paths'].values()),'Swagger operations')

expected={(m.upper(),p,c) for p,ops in s['paths'].items() for m in ops for c in ('no-token','wrong-organization','missing-body','invalid-body','empty-data','valid')}
measured={(x['method'],x['route'],x['case']) for x in rows if x['case']!='setup'}
assert measured==expected, 'Incomplete route/case coverage'
assert not any(x['status']=='ERROR' for x in rows), 'HTTP transport failure'

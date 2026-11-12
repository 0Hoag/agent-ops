import json,urllib.request,urllib.error,re,pathlib
ROOT=pathlib.Path(__file__).resolve().parents[2]
OUT=ROOT/'.agent-runs/W18R1-api-501'
OUT.mkdir(parents=True,exist_ok=True)
BASE='http://127.0.0.1:35081'
rows=[]
def call(m,p,body=None,token=None,extra=None,case='setup',route=None):
 h={'Content-Type':'application/json'}
 if token:h['Authorization']='Bearer '+token
 h.update(extra or {})
 raw=body if isinstance(body,bytes) else json.dumps(body).encode() if body is not None else None
 req=urllib.request.Request(BASE+p,data=raw,headers=h,method=m)
 try:
  with urllib.request.urlopen(req,timeout=8) as r:status=r.status; data=r.read().decode()
 except urllib.error.HTTPError as e:status=e.code;data=e.read().decode()
 try:v=json.loads(data)
 except ValueError:v=data
 shape=','.join(sorted(v)) if isinstance(v,dict) else type(v).__name__
 rows.append(dict(method=m,route=route or p,path=p,case=case,request=body.decode() if isinstance(body,bytes) else body,status=status,shape=shape,response=v))
 (OUT/'requests-progress.json').write_text(json.dumps(rows,indent=2))
 return v

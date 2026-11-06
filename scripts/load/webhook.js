import http from 'k6/http';
import { check } from 'k6';
import { hmac } from 'k6/crypto';
const target=__ENV.WEBHOOK_URL; const secret=__ENV.WEBHOOK_SECRET; const repository=__ENV.WEBHOOK_REPOSITORY;
export const options={scenarios:{webhook:{executor:'constant-arrival-rate',rate:Number(__ENV.WEBHOOK_RATE||50),timeUnit:'1s',duration:__ENV.WEBHOOK_DURATION||'30s',preAllocatedVUs:100,maxVUs:250}},thresholds:{http_req_failed:['rate==0'],checks:['rate==1'],dropped_iterations:['count==0']}};
export default function(){const id=__VU*1000000+__ITER+1;const body=JSON.stringify({action:'opened',repository:{full_name:repository},issue:{id:1600000000+id,number:id,title:`W16 load fixture issue ${id}`,state:'open',labels:[]}});const sig=`sha256=${hmac('sha256',secret,body,'hex')}`;const r=http.post(target,body,{headers:{'Content-Type':'application/json','X-GitHub-Delivery':`w16i-${id}`,'X-GitHub-Event':'issues','X-Hub-Signature-256':sig}});check(r,{'webhook returned 2xx':x=>x.status>=200&&x.status<300,'webhook accepted persisted delivery':x=>x.status===200&&JSON.parse(x.body).data.accepted===true});}

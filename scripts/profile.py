"""Measure Go API latency/RSS. Does not claim dataset accuracy.
Use --api-pid for native runs, or container metrics separately.
"""
import argparse,json,threading,time,urllib.request
from pathlib import Path
import psutil

p=argparse.ArgumentParser();p.add_argument('--api',default='http://localhost:8080');p.add_argument('--api-pid',type=int,required=True);p.add_argument('--records',type=int,default=1000);p.add_argument('--output',required=True)
a=p.parse_args();stop=threading.Event();peaks={str(pid):0 for pid in [a.api_pid]}
processes=[psutil.Process(pid) for pid in [a.api_pid]]
cpu_start=[sum(pr.cpu_times()[:2]) for pr in processes]
def sample():
    while not stop.wait(.05):
        for pr in processes:
            try:peaks[str(pr.pid)]=max(peaks[str(pr.pid)],pr.memory_info().rss)
            except psutil.NoSuchProcess:pass
sampler=threading.Thread(target=sample);sampler.start()
examples=['Алматы, улица Новая {n}а кв 4','Астана, проспект Туран {n}/6 магазин Береке 4подьезд 9этаж','Караганда, 66квартал {n}дом 69квартира','Шымкент, улица Абая {n} кв 12']
records=[{'id':str(i),'raw_address':examples[i%len(examples)].format(n=i+1)} for i in range(a.records)]
start=time.monotonic()
try:
    req=urllib.request.Request(a.api+'/v1/normalize',data=json.dumps({'records':records}).encode(),headers={'Content-Type':'application/json'})
    with urllib.request.urlopen(req,timeout=310) as r:result=json.load(r)
finally:
    stop.set();sampler.join()
elapsed=time.monotonic()-start
assert len(result['results'])==a.records
assert [r['id'] for r in result['results']]==[r['id'] for r in records]
warnings=sorted({w for r in result['results'] for w in r['warnings']})
assert 'libpostal_unavailable' not in warnings,warnings
report={'records':a.records,'wall_seconds':elapsed,'records_per_second':a.records/elapsed,
        'cpu_seconds':{str(pr.pid):sum(pr.cpu_times()[:2])-initial for pr,initial in zip(processes,cpu_start)},
        'peak_sampled_rss_bytes':peaks,'warnings':warnings,'within_5_minutes':elapsed<=300,
        'scope':'Native local CPU integration; varied synthetic texts, not accuracy evaluation. RSS sampled every 50ms; startup peaks measured separately.'}
Path(a.output).write_text(json.dumps(report,indent=2)+'\n');print(json.dumps(report,indent=2))
if elapsed>300:raise SystemExit('CPU performance target missed')

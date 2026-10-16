import json, logging, os, time
from contextlib import asynccontextmanager
from fastapi import FastAPI, HTTPException, Request
from starlette.concurrency import run_in_threadpool
from runtime import Runtime

@asynccontextmanager
async def lifespan(app):
    start=time.monotonic()
    app.state.runtime=Runtime(os.environ.get('GLINER_MODEL_PATH','/models/gliner'),
        threads=int(os.environ.get('GLINER_THREADS','2')),microbatch=int(os.environ.get('GLINER_MICROBATCH','8')),
        quantize=os.environ.get('GLINER_QUANTIZE','true')=='true')
    logging.info('GLiNER CPU ready in %.2fs',time.monotonic()-start)
    yield
app=FastAPI(lifespan=lifespan,docs_url=None,redoc_url=None,openapi_url=None)

@app.get('/readyz')
def ready():return {'status':'ok','device':'cpu'}

@app.post('/v1/entities')
async def entities(request:Request):
    # Bound actual streamed bytes, not merely the untrusted Content-Length.
    body=bytearray()
    async for chunk in request.stream():
        body.extend(chunk)
        if len(body)>2*1024*1024:raise HTTPException(413,'request exceeds 2 MiB')
    try:
        payload=json.loads(body);texts=payload['texts']
        if set(payload)!={'texts'} or not isinstance(texts,list) or not 1<=len(texts)<=1000:raise ValueError()
        if any(not isinstance(t,str) or len(t.encode())>4096 for t in texts):raise ValueError()
    except (ValueError,KeyError,TypeError):raise HTTPException(400,'expected {texts: [1..1000 strings, each <=4096 bytes]}')
    # Slice long texts into overlapping windows so no tail is silently truncated.
    # This is character-based; conservatively small windows keep mixed-script
    # addresses well below encoder token limits. Spans remain Python codepoints.
    windows=[];mapping=[]
    for i,text in enumerate(texts):
        if not text:continue
        for start in range(0,len(text),192):
            windows.append(text[start:start+256]);mapping.append((i,start))
            if start+256>=len(text):break
    results=[[] for _ in texts]
    if windows:
        rows=await run_in_threadpool(request.app.state.runtime.predict,windows)
        if len(rows)!=len(windows):raise HTTPException(500,'model returned wrong window count')
        for row,(index,offset) in zip(rows,mapping):
            for e in row:
                e={**e,'start':e['start']+offset,'end':e['end']+offset}
                if texts[index][e['start']:e['end']]!=e['text']:raise HTTPException(500,'model returned invalid span')
                results[index].append(e)
    for i,row in enumerate(results):
        unique={}
        for e in row:
            key=(e['start'],e['end'],e['label'])
            if key not in unique or unique[key]['score']<e['score']:unique[key]=e
        results[i]=list(unique.values())
    return {'results':results}

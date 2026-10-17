import { test, expect } from '@playwright/test';

for (const width of [1440,390]) {
 test(`hidden CLI tabs release SSE and resume drafts at ${width}px`,async({page,context})=>{
  await page.setViewportSize({width,height:844});
  // Headless browsers do not consistently hide background pages. Control only
  // visibility; exercise the real HTTP/1.1 streams and connection pool.
  await context.addInitScript(()=>{
   let visibility='visible';
   Object.defineProperty(document,'visibilityState',{get:()=>visibility});
   window.testVisibility=value=>{visibility=value;document.dispatchEvent(new Event('visibilitychange'))};
   const Original=window.EventSource;window.testStreams=[];
   window.EventSource=class extends Original{
    constructor(...args){super(...args);window.testStreams.push(this)}
   };
  });
  const created=await page.request.post('/v1/runs/input',{data:{records:[{id:'visibility-a',raw_address:'Синтетический адрес A'},{id:'visibility-b',raw_address:'Синтетический адрес B'}]}});
  expect(created.ok()).toBe(true);const run=await created.json();
  await expect.poll(async()=> (await (await page.request.get(`/v1/runs/${run.id}`)).json()).status,{timeout:120000}).toBe('review');
  await page.goto(`/?run=${run.id}`);
  await expect(page.locator('#rows > tr')).toHaveCount(2);
  await page.locator('#rows > tr').first().getByRole('button').click();
  await page.getByLabel('Квартира',{exact:true}).fill('Мой черновик');
  await page.evaluate(()=>window.testVisibility('hidden'));
  await expect.poll(()=>page.evaluate(()=>window.testStreams.every(s=>s.readyState===EventSource.CLOSED))).toBe(true);
  // Six background tabs previously consumed every connection and blocked a
  // seventh page. Now each hidden tab releases its stream before the next opens.
  const extras=[];
  try{
   for(let i=0;i<6;i++){
    const extra=await context.newPage();extras.push(extra);
    await extra.goto('/?new=true',{timeout:10000});
    await expect(extra.getByRole('heading',{name:'Нормализация адресов',exact:true})).toBeVisible();
    await extra.evaluate(()=>window.testVisibility('hidden'));
    await expect.poll(()=>extra.evaluate(()=>window.testStreams.every(s=>s.readyState===EventSource.CLOSED))).toBe(true);
   }
   const foreground=await context.newPage();extras.push(foreground);
   await foreground.goto(`/?job=${run.id}`,{timeout:10000});
   await expect(foreground.locator('#rows > tr')).toHaveCount(2);
   expect(await foreground.evaluate(async()=>{const r=await fetch('/healthz',{signal:AbortSignal.timeout(2000)});return r.ok})).toBe(true);
   await foreground.evaluate(()=>window.testVisibility('hidden'));
   const rows=(await(await page.request.get(`/v1/runs/${run.id}/rows`)).json()).rows;
   const other=rows[1];const saved=await page.request.post(`/v1/runs/${run.id}/rows/${other.position}/review`,{data:{expected_revision:other.revision,fields:{apartment:'88'}}});expect(saved.ok()).toBe(true);
   const reconnected=page.waitForResponse(r=>r.url().endsWith('/v1/events')&&r.status()===200);
   await page.evaluate(()=>window.testVisibility('visible'));await reconnected;
   await expect(page.getByLabel('Квартира',{exact:true})).toHaveValue('Мой черновик');
   page.once('dialog',d=>d.accept());await page.getByRole('button',{name:'Свернуть',exact:true}).click();
   await expect(page.getByLabel('Итоги запуска').locator('div').filter({hasText:'Требуют проверки'}).locator('dd')).toHaveText('1');
   await page.locator('#rows tr').nth(1).getByRole('button').click();
   await expect(page.getByLabel('Квартира',{exact:true})).toHaveValue('88');
   await page.evaluate(()=>window.dispatchEvent(new PageTransitionEvent('pagehide',{persisted:true})));
   await expect.poll(()=>page.evaluate(()=>window.testStreams.every(s=>s.readyState===EventSource.CLOSED))).toBe(true);
   const restored=page.waitForResponse(r=>r.url().endsWith('/v1/events')&&r.status()===200);
   await page.evaluate(()=>window.dispatchEvent(new PageTransitionEvent('pageshow',{persisted:true})));await restored;
  }finally{await Promise.all(extras.map(p=>p.close()))}
 });
}

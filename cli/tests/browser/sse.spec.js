import {test,expect} from '@playwright/test';
for(const viewport of [{width:1440,height:1000},{width:390,height:844}]) {
 test(`root loads CLI journal and SSE preserves drafts at ${viewport.width}px`,async({page})=>{
  await page.setViewportSize(viewport);
  const created=await page.request.post('/v1/runs/input',{data:{records:[{id:'sse-first',raw_address:'Синтетический адрес 1'},{id:'sse-second',raw_address:'Синтетический адрес 2'}]}});
  expect(created.ok()).toBe(true);const run=await created.json();
  const stream=page.waitForResponse(r=>r.url().endsWith('/v1/events')&&r.status()===200);
  await page.goto('/');await stream;
  await expect(page).toHaveURL(new RegExp(`[?&]job=${run.id}`));
  await expect(page.getByLabel('Итоги запуска').locator('div').filter({hasText:'Требуют проверки'}).locator('dd')).toHaveText('2',{timeout:120000});
  await expect(page.locator('#rows tr')).toHaveCount(2);
  await page.locator('#rows tr').first().getByRole('button').click();
  await page.getByLabel('Квартира',{exact:true}).fill('черновик');
  const data=await (await page.request.get(`/v1/runs/${run.id}/rows`)).json();const other=data.rows[1];
  const saved=await page.request.post(`/v1/runs/${run.id}/rows/${other.position}/review`,{data:{expected_revision:other.revision,fields:{apartment:'88'}}});expect(saved.ok()).toBe(true);
  await expect(page.getByLabel('Квартира',{exact:true})).toHaveValue('черновик');
  page.once('dialog',d=>d.accept());await page.getByRole('button',{name:'Свернуть',exact:true}).click();
  // No reload or manual refresh: the queued SSE revision applies on closing the editor.
  await expect(page.getByLabel('Итоги запуска').locator('div').filter({hasText:'Требуют проверки'}).locator('dd')).toHaveText('1');
  await page.locator('#rows tr').nth(1).getByRole('button').click();
  await expect(page.getByLabel('Квартира',{exact:true})).toHaveValue('88');
  await page.getByLabel('Подъезд',{exact:true}).fill('3');
  await page.getByRole('button',{name:'Подтвердить адрес',exact:true}).click();
  await expect(page.locator('#editor')).toBeVisible();
  await expect(page.getByRole('button',{name:'Подтвердить адрес',exact:true})).toBeEnabled();
  const persisted=await (await page.request.get(`/v1/runs/${run.id}/rows`)).json();expect(persisted.rows[1].result.entrance).toBe('3');
  await page.reload();await expect(page.locator('#rows tr')).toHaveCount(2);
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
 });
}

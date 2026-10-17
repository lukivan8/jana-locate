import { test, expect } from '@playwright/test';
import { execFileSync } from 'node:child_process';
for (const viewport of [{width:1440,height:1000},{width:390,height:844}]) {
 test(`partial CLI publication remains open for normalize at ${viewport.width}px`, async ({page}) => {
  test.skip(!process.env.JANA_TEST_WORKSPACE_CONTAINER, 'requires the disposable CLI Docker fixture');
  await page.setViewportSize(viewport);
  const response=await page.request.post('/v1/runs',{data:{schema:'public',table:'orders',key:'id',column:'address',target_schema:'public',target_table:`partial_${viewport.width}_addresses`}});
  expect(response.ok()).toBe(true);const created=await response.json();
  await expect.poll(async()=> (await (await page.request.get(`/v1/runs/${created.id}`)).json()).status,{timeout:120000}).toBe('review');
  const rows=(await (await page.request.get(`/v1/runs/${created.id}/rows`)).json()).rows;
  const first=rows[0];const fields={};
  for(const name of ['province','city','district','living_area','street_type','street_name','house','apartment','entrance','floor','landmark'])fields[name]=first.result[name];
  fields.apartment='88';
  const reviewed=await page.request.post(`/v1/runs/${created.id}/rows/${first.position}/review`,{data:{expected_revision:first.revision,fields}});
  expect(reviewed.ok()).toBe(true);
  const publish=()=>JSON.parse(execFileSync('docker',['exec',process.env.JANA_TEST_WORKSPACE_CONTAINER,'/jana','normalize','--run',created.id,'--confirm','--quiet'],{encoding:'utf8',stdio:['ignore','pipe','pipe']}));
  const firstPublication=publish();
  expect(firstPublication.status).toBe('review');expect(firstPublication.published_count).toBe(1);
  await page.goto(`/?job=${created.id}`);
  await expect(page.getByRole('heading',{name:'Очередь проверки'})).toBeVisible();
  await expect(page.getByRole('heading',{name:'Итоговая таблица'})).toHaveCount(0);
  await page.getByLabel('Только требующие проверки',{exact:true}).check();
  await expect(page.locator('#rows > tr:not(.review-details-row)')).toHaveCount(4);
  await page.locator('#rows > tr:not(.review-details-row)').first().getByRole('button').click();
  await page.getByLabel('Квартира',{exact:true}).fill('99');
  await page.getByRole('button',{name:'Подтвердить адрес',exact:true}).click();
  await expect(page.locator('#editor')).toBeVisible();
  await expect(page.getByRole('button',{name:'Подтвердить адрес',exact:true})).toBeEnabled();
  const secondPublication=publish();
  expect(secondPublication.id).toBe(created.id);expect(secondPublication.status).toBe('review');expect(secondPublication.published_count).toBe(2);
  await page.reload();
  await expect(page.getByRole('heading',{name:'Очередь проверки'})).toBeVisible();
  await page.getByLabel('Только требующие проверки',{exact:true}).check();
  await expect(page.locator('#rows > tr:not(.review-details-row)')).toHaveCount(3);
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
 });
}

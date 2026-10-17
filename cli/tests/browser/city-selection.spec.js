import {test,expect} from '@playwright/test';
for(const width of [1440,390]){
 test(`CLI uses shared settlement picker and original row position at ${width}px`,async({page,context})=>{
  await page.setViewportSize({width,height:width===390?844:1000});
  const raw='  Караганда, улица Алалыкина 999999  ';
  const records=Array.from({length:51},(_,i)=>({id:'duplicate',raw_address:i===50?'Караганда':`Синтетический неизвестный адрес ${i}`}));
  const body=JSON.stringify({records}).slice(0,-2)+`,{"id":9223372036854775807,"raw_address":${JSON.stringify(raw)}}]}`;
  const response=await page.request.post('/v1/runs/input',{data:body,headers:{'Content-Type':'application/json'}});
  expect(response.ok()).toBe(true);const run=await response.json();
  await expect.poll(async()=>(await(await page.request.get(`/v1/runs/${run.id}`)).json()).status,{timeout:120000}).toBe('review');
  const url=`/?run=${run.id}`;
  await page.goto(url);await expect(page.locator('#rows > tr')).toHaveCount(50);
  await page.getByRole('button',{name:'Далее',exact:true}).click();await expect(page.locator('#rows > tr')).toHaveCount(2);
  await page.locator('#rows > tr').nth(1).getByRole('button').click();
  const picker=page.getByRole('region',{name:'Населённый пункт по справочнику',exact:true});
  await expect(picker.getByRole('radio')).toHaveCount(2);
  await expect(picker.getByRole('radio',{name:/Караганда · город.*Карагандинская область/})).toBeVisible();
  await expect(picker.getByRole('radio',{name:/Караганда · село.*Актюбинская область/})).toBeVisible();
  const other=await context.newPage();
  try{
   await other.goto(url);await expect(other.locator('#rows > tr')).toHaveCount(50);
   await other.getByRole('button',{name:'Далее',exact:true}).click();await expect(other.locator('#rows > tr')).toHaveCount(2);
   await other.locator('#rows > tr').nth(1).getByRole('button').click();
   await other.getByRole('radio',{name:/Караганда · село/}).check();
   await picker.getByRole('radio',{name:/Караганда · город/}).check();
   await picker.getByRole('button',{name:'Выбрать населённый пункт',exact:true}).click();
   await expect(page.getByLabel('Область',{exact:true})).toHaveValue('Карагандинская область');
   await expect(picker.getByRole('button',{name:'Исправить',exact:true})).toBeVisible();
   await other.getByRole('button',{name:'Выбрать населённый пункт',exact:true}).click();
   await expect(other.getByRole('alert')).toContainText(/Версия|изменил|Задание/);
  }finally{await other.close()}
  const rows=(await(await page.request.get(`/v1/runs/${run.id}/rows?offset=50`)).json()).rows;
  expect(rows[0].city_selection).toBeUndefined();expect(rows[1].city_selection.record_index).toBe(51);
  expect(rows[1].reviewed).toBe(false);expect(rows[1].input_id).toBe('9223372036854775807');expect(rows[1].raw_address).toBe(raw);
  expect(rows[1].result.coordinates).toBeNull();expect(rows[1].result.warnings).toContain('city_selected_by_operator');
  await page.reload();await expect(page.locator('#rows > tr')).toHaveCount(50);
  await page.getByRole('button',{name:'Далее',exact:true}).click();await expect(page.locator('#rows > tr')).toHaveCount(2);
  await page.locator('#rows > tr').nth(1).getByRole('button').click();
  await expect(picker).toContainText('Карагандинская область');await picker.getByRole('button',{name:'Исправить',exact:true}).click();
  await expect(picker.getByRole('radio',{name:/Караганда · город/})).toBeChecked();
  await picker.getByRole('radio',{name:/Караганда · село/}).check();
  await picker.getByRole('button',{name:'Выбрать населённый пункт',exact:true}).click();
  await expect(page.getByLabel('Область',{exact:true})).toHaveValue('Актюбинская область');
  const updated=await(await page.request.get(`/v1/runs/${run.id}/rows?offset=50`)).json();expect(updated.rows[1].reviewed).toBe(false);
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
 });
}

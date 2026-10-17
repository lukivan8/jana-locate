import { test, expect } from '@playwright/test';
for (const width of [1440,390]) {
 test(`web connection catalog and saved local profile at ${width}px`,async({page})=>{
  await page.setViewportSize({width,height:width===390?844:1000});await page.goto('/?connect=true');
  await expect(page.getByRole('heading',{name:'Подключение PostgreSQL'})).toBeVisible();
  await page.getByLabel('Хост',{exact:true}).fill('postgres');
  await page.getByLabel('База данных',{exact:true}).fill('jana_demo');
  await page.getByLabel('Пользователь',{exact:true}).fill('jana_demo');
  await page.getByLabel('Пароль',{exact:true}).fill('wrong');
  await page.getByRole('button',{name:'Проверить подключение',exact:true}).click();
  await expect(page.getByRole('alert')).toContainText('Подключение не прошло');
  await page.getByLabel('Пароль',{exact:true}).fill('synthetic-only');
  await page.getByRole('button',{name:'Проверить подключение',exact:true}).click();
  await expect(page.getByRole('status').filter({hasText:/^Подключение PostgreSQL установлено\.$/})).toBeVisible();
  await page.getByLabel('Таблица',{exact:true}).selectOption({label:'public.orders'});
  await expect(page.getByLabel('Колонка адреса',{exact:true})).toHaveValue('address');
  await expect(page.getByLabel('Первичный ключ',{exact:true})).toHaveValue('id');
  await page.getByLabel('Таблица результата',{exact:true}).fill(`connect_${width}_result`);
  const pending=page.waitForResponse(response=>response.url().endsWith('/v1/connection/save'));
  await page.getByRole('button',{name:'Сохранить подключение',exact:true}).click();
  const response=await pending;expect(response.ok()).toBe(true);expect(await response.text()).not.toContain('synthetic-only');
  await expect(page.getByRole('status')).toHaveText('Подключение проверено и сохранено локально.');
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
  await page.reload();await expect(page.getByLabel('Пароль',{exact:true})).toHaveValue('');
  await expect(page.getByLabel('Хост',{exact:true})).toHaveValue('postgres');
  await expect(page.getByLabel('База данных',{exact:true})).toHaveValue('jana_demo');
  await expect(page.getByLabel('Пользователь',{exact:true})).toHaveValue('jana_demo');
  await expect(page.getByRole('status').filter({hasText:'Сохранённое подключение'})).toContainText(`public.orders.address → public.connect_${width}_result`);
  await expect(page.getByLabel('Использовать сохранённый пароль, если поле пустое')).toBeChecked();
  const profile = await (await page.request.get('/v1/connection')).text();
  expect(profile).not.toContain('synthetic-only');expect(profile).not.toContain('postgresql://');
  await page.getByRole('button',{name:'Проверить подключение',exact:true}).click();
  await expect(page.getByLabel('Таблица',{exact:true}).locator('option:checked')).toHaveText('public.orders');
  await expect(page.getByLabel('Колонка адреса',{exact:true})).toHaveValue('address');
  await expect(page.getByLabel('Таблица результата',{exact:true})).toHaveValue(`connect_${width}_result`);
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
 });
}

import { useEffect, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { Button, SelectField, TextField } from '../../ui/material'
import { connectionProfile, inspectDatabase, saveConnection, type ConnectionProfile, isLocalWorkspace, type ConnectionRequest, type DatabaseCatalog } from '../../api/client'

export function ConnectWorkspace() {
  const [credentials, setCredentials] = useState<ConnectionRequest>({ host: 'localhost', port: 5432, database: 'postgres', user: 'postgres', password: '', sslmode: 'prefer' })
  const [catalog, setCatalog] = useState<DatabaseCatalog | null>(null)
  const [tableIndex, setTableIndex] = useState(0)
  const [column, setColumn] = useState('')
  const [targetSchema, setTargetSchema] = useState('public')
  const [targetTable, setTargetTable] = useState('')
  const [busy, setBusy] = useState(false)
  const [saved, setSaved] = useState(false)
  const [error, setError] = useState('')
  const [profile, setProfile] = useState<ConnectionProfile | null>(null)
  const [initialized, setInitialized] = useState(false)
  const [loadAttempt, setLoadAttempt] = useState(0)
  useEffect(() => {
    if (!isLocalWorkspace()) return
    let active = true
    setInitialized(false); setError('')
    void connectionProfile().then(value => {
      if (!active) return
      setProfile(value)
      if (value.configured) {
        setCredentials({ host: value.host, port: value.port, database: value.database, user: value.user, sslmode: value.sslmode, password: '', use_saved_password: value.has_password })
        setTargetSchema(value.source?.target_schema ?? 'public')
      }
      setInitialized(true)
    }).catch(e => { if (active) setError(e instanceof Error ? e.message : String(e)) })
    return () => { active = false }
  }, [loadAttempt])
  const table = catalog?.tables[tableIndex]
  const sameConnection = profile?.configured && credentials.host === profile.host && credentials.port === profile.port && credentials.database === profile.database && credentials.user === profile.user
  function chooseTable(result: DatabaseCatalog, index: number) {
    const selected = result.tables[index]
    const previous = sameConnection && profile.source?.schema === selected?.schema && profile.source?.table === selected?.name ? profile.source : undefined
    setTableIndex(index); setColumn(previous && selected.columns.includes(previous.column) ? previous.column : selected?.columns[0] ?? '')
    setTargetSchema(previous?.target_schema ?? 'public'); setTargetTable(previous?.target_table ?? `${selected?.name ?? ''}_jana_addresses`)
  }
  async function check() {
    setBusy(true); setError('')
    try { const result = await inspectDatabase(credentials); setCatalog(result); const index = sameConnection ? result.tables.findIndex(value => value.schema === profile.source?.schema && value.name === profile.source?.table) : -1; chooseTable(result, Math.max(0, index)) }
    catch (e) { setError(e instanceof Error ? e.message : String(e)) }
    finally { setBusy(false) }
  }
  async function save() {
    if (!table) return
    setBusy(true); setError('')
    try { await saveConnection({ ...credentials, source: { schema: table.schema, table: table.name, key: table.key, column, target_schema: targetSchema, target_table: targetTable } }); setCredentials({ ...credentials, password: '' }); setCatalog(null); setSaved(true) }
    catch (e) { setError(e instanceof Error ? e.message : String(e)) }
    finally { setBusy(false) }
  }
  if (!isLocalWorkspace()) return <main id="main"><h1>Подключение PostgreSQL</h1><p>Запустите локальный CLI: <code>jana connect --web</code>.</p></main>
  return <main id="main" className="address-workspace"><section className="panel"><h1>Подключение PostgreSQL</h1>
    {saved ? <><p role="status">Подключение проверено и сохранено локально.</p><p>Вернитесь в терминал и запустите <code>jana normalize</code>.</p><Link to="/" search={{}}>Открыть запуски</Link></> : <>
      <p className="muted">Доступы сохраняются в профиле CLI. Нормализатор получает только адреса. Хост указывается относительно сервиса CLI.</p>
      {profile?.configured && <div role="status" className="message connection-summary"><strong>Сохранённое подключение</strong><p>{profile.user}@{profile.host}:{profile.port} / {profile.database}</p>{profile.source && <p>{profile.source.schema}.{profile.source.table}.{profile.source.column} → {profile.source.target_schema}.{profile.source.target_table}</p>}<p className="muted">Последняя проверка: {new Date(profile.verified_at).toLocaleString('ru-RU')}. Параметры подставлены из профиля; при продолжении подключение проверится заново.</p></div>}
      {!initialized ? <>{error ? <Button onClick={() => setLoadAttempt(value => value + 1)}>Повторить загрузку профиля</Button> : <p role="status">Загружаем параметры подключения…</p>}</> : !catalog ? <form className="connection-form" onSubmit={event => { event.preventDefault(); void check() }}><div className="field-grid">
        {(['host', 'database', 'user'] as const).map(key => <TextField key={key} label={{ host: 'Хост', database: 'База данных', user: 'Пользователь' }[key]} value={credentials[key]} required disabled={busy} autoComplete="off" onChange={event => setCredentials({ ...credentials, [key]: event.target.value, use_saved_password: false })} />)}
        <TextField label="Порт" type="number" min={1} max={65535} required value={credentials.port} disabled={busy} onChange={event => setCredentials({ ...credentials, port: Number(event.target.value), use_saved_password: false })} />
        <TextField label="Пароль" supportingText={credentials.use_saved_password ? 'Пустое поле сохраняет пароль текущего подключения.' : undefined} type="password" autoComplete="off" value={credentials.password} disabled={busy} onChange={event => setCredentials({ ...credentials, password: event.target.value })} />
        <SelectField label="Режим SSL" value={credentials.sslmode} disabled={busy} onChange={event => setCredentials({ ...credentials, sslmode: event.target.value })}>{['prefer', 'require', 'verify-full', 'verify-ca', 'disable', 'allow'].map(mode => <option key={mode} value={mode}>{{ prefer: 'Предпочитать SSL', require: 'Требовать SSL', 'verify-full': 'Проверять сертификат и хост', 'verify-ca': 'Проверять сертификат', disable: 'Без SSL', allow: 'Допускать SSL' }[mode]} ({mode})</option>)}</SelectField>
      </div>{sameConnection && profile.has_password && <label><input type="checkbox" checked={!!credentials.use_saved_password} disabled={busy} onChange={event => setCredentials({ ...credentials, use_saved_password: event.target.checked })} /> Использовать сохранённый пароль, если поле пустое</label>}<div className="actions"><Button variant="filled" type="submit" disabled={busy}>{busy ? 'Проверяем подключение…' : 'Проверить подключение'}</Button></div></form> : <>
        <p role="status">Подключение PostgreSQL установлено.</p>
        {!catalog.tables.length ? <p>Нет доступных таблиц с одиночным первичным ключом и текстовыми колонками. Проверьте права SELECT и структуру базы.</p> : <form className="connection-form" onSubmit={event => { event.preventDefault(); void save() }}><div className="field-grid">
          <SelectField label="Таблица" disabled={busy} value={tableIndex} onChange={event => { chooseTable(catalog, Number(event.target.value)) }}>{catalog.tables.map((value, index) => <option key={`${value.schema}.${value.name}`} value={index}>{value.schema}.{value.name}</option>)}</SelectField>
          <SelectField label="Колонка адреса" value={column} disabled={busy} onChange={event => setColumn(event.target.value)}>{table?.columns.map(name => <option key={name}>{name}</option>)}</SelectField>
          <TextField label="Первичный ключ" value={table?.key ?? ''} readOnly />
          <TextField label="Схема результата" value={targetSchema} required maxLength={63} disabled={busy} onChange={event => setTargetSchema(event.target.value)} />
          <TextField label="Таблица результата" value={targetTable} required maxLength={63} disabled={busy} onChange={event => setTargetTable(event.target.value)} />
        </div><Button variant="filled" type="submit" disabled={busy}>{busy ? 'Сохраняем…' : 'Сохранить подключение'}</Button></form>}
        <Button variant="text" disabled={busy} onClick={() => { setCatalog(null); setError('') }}>Изменить доступы</Button>
      </>}
      {error && <p role="alert" className="message error">{error}</p>}
    </>}
  </section></main>
}

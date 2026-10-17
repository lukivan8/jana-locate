import { useEffect, useState } from 'react'
import { RunTiming } from './run-timing'
import { Button } from '../../ui/material'
import { publishedTable, type LocalRun, type PublishedTable } from '../../api/client'

export function PublishedWorkspace({ run, onNewRun }: { run: LocalRun; onNewRun: () => void }) {
  const [offset, setOffset] = useState(0)
  const [table, setTable] = useState<PublishedTable | null>(null)
  const [error, setError] = useState('')
  const [attempt, setAttempt] = useState(0)
  useEffect(() => {
    let active = true
    setTable(null); setError('')
    void publishedTable(run.id, offset).then(data => { if (active) setTable(data) }).catch(e => { if (active) setError(e instanceof Error ? e.message : String(e)) })
    return () => { active = false }
  }, [run.id, offset, attempt])
  const columns = table ? table.columns.map((name, index) => ({ name, index })).sort((a, b) => Number(a.name === 'run_id') - Number(b.name === 'run_id')) : []
  return <main id="main" className="address-workspace"><section className="panel results"><div className="section-title"><h1>Итоговая таблица</h1><Button variant="filled" onClick={onNewRun}>Новый запуск</Button></div><p id="published" role="status">Результат опубликован в {run.source.target_schema}.{run.source.target_table}.</p><p className="muted">Записанные строки запуска {run.id}. Данные читаются из PostgreSQL.</p>
    <RunTiming run={run} />
    {error ? <><p role="alert" className="message error">{error}</p><Button onClick={() => setAttempt(value => value + 1)}>Повторить загрузку</Button></> : !table ? <p role="status">Загружаем итоговую таблицу…</p> : <>
      <p>Всего строк: {table.total}. Таблица прокручивается по горизонтали.</p>
      <div className="table-scroll" role="region" aria-label="Итоговая таблица PostgreSQL" tabIndex={0}><table><thead><tr>{columns.map(({ name }) => <th key={name} scope="col">{name}</th>)}</tr></thead><tbody>{table.rows.map((row, i) => <tr key={offset + i}>{columns.map(({ index }) => <td key={index}>{row[index]}</td>)}</tr>)}</tbody></table></div>
      <nav className="actions pagination" aria-label="Страницы итоговой таблицы"><Button disabled={offset === 0} onClick={() => setOffset(value => Math.max(0, value - 50))}>Назад</Button><span>{table.total ? `${offset + 1}–${Math.min(offset + 50, table.total)} из ${table.total}` : 'Записей нет'}</span><Button disabled={offset + 50 >= table.total} onClick={() => setOffset(value => value + 50)}>Далее</Button></nav>
    </>}
  </section></main>
}

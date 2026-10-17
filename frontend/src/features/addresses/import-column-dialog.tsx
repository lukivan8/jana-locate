import { useEffect, useRef, useState } from 'react'
import { Button } from '../../ui/material'
import { addressesFromColumn } from './import-addresses'
import type { InputRecord } from '../../api/client'

export function ImportColumnDialog({ rows, name, sheetName, onImport, onCancel }: {
  rows: string[][]; name: string; sheetName?: string
  onImport: (records: InputRecord[]) => void; onCancel: () => void
}) {
  const dialog = useRef<HTMLDialogElement>(null)
  const [column, setColumn] = useState(() => Math.max(0, rows[0].findIndex(cell => cell.trim().toLowerCase() === 'raw_address')))
  const [hasHeader, setHasHeader] = useState(true)
  const [error, setError] = useState('')
  useEffect(() => {
    const element = dialog.current!
    const previous = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    element.showModal()
    return () => { element.close(); document.body.style.overflow = previous }
  }, [])
  return <dialog ref={dialog} className="import-column-dialog" aria-labelledby="import-column-title" aria-describedby="import-column-description" onCancel={event => { event.preventDefault(); onCancel() }}>
    <form onSubmit={event => {
      event.preventDefault()
      try { onImport(addressesFromColumn(rows, column, hasHeader)) }
      catch (err) { setError(err instanceof Error ? err.message : String(err)) }
    }}>
      <h2 id="import-column-title">Выберите колонку с адресами</h2>
      <p id="import-column-description" className="muted">{name}{sheetName && ` · первый лист: ${sheetName}`}. В поле ввода попадут значения выбранной колонки.</p>
      <label className="import-header-option"><input type="checkbox" checked={hasHeader} onChange={event => { setHasHeader(event.target.checked); setError('') }} />Первая строка — заголовки</label>
      <fieldset className="import-column-options"><legend>Колонка адресов</legend>
        {rows[0].map((cell, index) => <label key={index} className="import-column-option">
          <input type="radio" name="address-column" checked={column === index} onChange={() => { setColumn(index); setError('') }} />
          <span><strong>{hasHeader && cell.trim() ? cell : `Колонка ${index + 1}`}</strong><small>{rows.slice(hasHeader ? 1 : 0, hasHeader ? 4 : 3).map(row => row[index] || 'Пустая ячейка').join(' · ') || 'Нет значений'}</small></span>
        </label>)}
      </fieldset>
      {error && <p role="alert" className="message error">{error}</p>}
      <div className="actions import-column-actions"><Button variant="tonal" onClick={onCancel}>Отмена</Button><Button variant="filled" type="submit">Импортировать</Button></div>
    </form>
  </dialog>
}

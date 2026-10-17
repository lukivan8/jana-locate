import { useEffect, useState } from 'react'
import { Link, Outlet } from '@tanstack/react-router'
import { isLocalWorkspace } from '../../api/client'

export function DemoLayout() {
  const [local, setLocal] = useState(false)
  useEffect(() => { setLocal(isLocalWorkspace()) }, [])
  return <div className="demo-shell">
    <a className="skip-link" href="#main">К содержимому</a>
    <header><Link className="brand" to="/">jana<span>locate</span></Link>
      <nav className="header-nav" aria-label="Сценарии демонстрации">
        {local ? <a href="/openapi.json">Документация</a> : <Link to="/api" activeProps={{ 'aria-current': 'page' }}>Документация</Link>}
        {local ? <a href="/?new=true">Тестовый запуск</a> : <Link to="/geocoder" activeProps={{ 'aria-current': 'page' }}>Тестовый запуск</Link>}
      </nav>
    </header>
    <Outlet />
    <footer className="shell-footer">Jana Locate · Офлайн-обработка · Независимый HTTP API</footer>
  </div>
}

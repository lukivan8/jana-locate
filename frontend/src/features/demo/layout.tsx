import { Link, Outlet } from '@tanstack/react-router'

export function DemoLayout() {
  return <div className="demo-shell">
    <a className="skip-link" href="#main">К содержимому</a>
    <header><Link className="brand" to="/">jana<span>locate</span></Link><span className="tag">ADDRESS INTELLIGENCE / API v1</span></header>
    <nav className="demo-nav" aria-label="Сценарии демонстрации">
      <Link to="/api" activeProps={{ 'aria-current': 'page' }}><span>1</span> API и архитектура</Link>
      <Link to="/" activeOptions={{ exact: true }} activeProps={{ 'aria-current': 'page' }}><span>2</span> Пакетная обработка</Link>
      <Link to="/geocoder" activeProps={{ 'aria-current': 'page' }}><span>3</span> Свободный ввод</Link>
    </nav>
    <Outlet />
    <footer className="shell-footer">Jana Locate · Офлайн-обработка · Независимый HTTP API</footer>
  </div>
}

import { createRootRoute, HeadContent, Scripts } from '@tanstack/react-router'
import '../styles.css'
import { DemoLayout } from '../features/demo/layout'
export const Route = createRootRoute({
  head: () => ({ meta: [{ charSet: 'utf-8' }, { name: 'viewport', content: 'width=device-width, initial-scale=1' }, { title: 'Jana Locate · Адресный сервис' }] }),
  component: () => <html lang="ru"><head><HeadContent /><script src="/config.js" /></head><body><DemoLayout /><Scripts /></body></html>,
  notFoundComponent: () => <main><h1>Page not found</h1><a href="/">Return to review</a></main>,
})

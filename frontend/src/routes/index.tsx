import { ClientOnly, createFileRoute } from '@tanstack/react-router'
import { AddressWorkspace } from '../features/addresses/workspace'
// Match the empty prerendered SPA shell before mounting browser-only feature state.
export const Route = createFileRoute('/')({ ssr: false, wrapInSuspense: true, component: () => <ClientOnly><AddressWorkspace /></ClientOnly> })

import { ClientOnly, createFileRoute } from '@tanstack/react-router'
import { ConnectWorkspace } from '../features/addresses/connect'
import { AddressWorkspace } from '../features/addresses/workspace'
// Match the empty prerendered SPA shell before mounting browser-only feature state.
export const Route = createFileRoute('/')({ validateSearch: (search: Record<string, unknown>): { job?: string; connect?: boolean; new?: boolean } => ({ new: search.new === true || search.new === 'true' ? true : undefined, connect: search.connect === true || search.connect === 'true' ? true : undefined, job: search.new === true || search.new === 'true' ? undefined : typeof search.job === 'string' ? search.job : typeof search.run === 'string' ? search.run : undefined }), ssr: false, wrapInSuspense: true, component: WorkspaceRoute })

function WorkspaceRoute() { const search = Route.useSearch(); return <ClientOnly>{search.connect ? <ConnectWorkspace /> : <AddressWorkspace />}</ClientOnly> }

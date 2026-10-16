import { ClientOnly, createFileRoute } from '@tanstack/react-router'
import { ApiDocs } from '../features/demo/api-docs'
export const Route = createFileRoute('/api')({ ssr: false, component: () => <ClientOnly><ApiDocs /></ClientOnly> })

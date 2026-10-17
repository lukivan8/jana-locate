import { ClientOnly, createFileRoute } from '@tanstack/react-router'
import { ExampleExplorer } from '../features/addresses/example-explorer'
export const Route = createFileRoute('/examples')({ ssr: false, component: () => <ClientOnly><ExampleExplorer /></ClientOnly> })

import { ClientOnly, createFileRoute } from '@tanstack/react-router'
import { Geocoder } from '../features/demo/geocoder'
export const Route = createFileRoute('/geocoder')({ ssr: false, component: () => <ClientOnly><Geocoder /></ClientOnly> })

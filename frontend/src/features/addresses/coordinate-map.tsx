import { useId } from 'react'
import type { Result } from '../../api/client'
import boundary from '../../data/kazakhstan.json'

// Natural Earth 1:110m, public domain. Geographic context, not a street basemap.
const project = (lon: number, lat: number) => [24 + (lon - 45) / 44 * 652, 350 - (lat - 39) / 18 * 300]
const outline = boundary.coordinates.map(ring => ring.map(([lon, lat], i) => `${i ? 'L' : 'M'}${project(lon, lat).join(' ')}`).join(' ') + ' Z').join(' ')
export function CoordinateMap({ results, onSelect }: { results: Result[]; onSelect?: (index: number) => void }) {
  const id = useId()
  const points = results.flatMap((r, index) => r.coordinates ? [{ ...r.coordinates, index, label: [r.city, r.street_name, r.house].filter(Boolean).join(', ') }] : [])
  return <div className="coordinate-map">
    <svg viewBox="0 0 700 390" role="img" aria-labelledby={id}>
      <title id={id}>Офлайн-карта Казахстана: {points.length} адресов с координатами</title>
      {[50, 60, 70, 80].map(lon => <g key={lon}><path className="map-grid" d={`M${project(lon, 39).join(' ')} L${project(lon, 57).join(' ')}`} /><text x={project(lon, 39)[0]} y="377" className="plot-label">{lon}° E</text></g>)}
      {[40, 45, 50, 55].map(lat => <g key={lat}><path className="map-grid" d={`M${project(45, lat).join(' ')} L${project(89, lat).join(' ')}`} /><text x="8" y={project(45, lat)[1] - 4} className="plot-label">{lat}° N</text></g>)}
      <path d={outline} className="map-land" />
      <text x="290" y="180" className="map-country">КАЗАХСТАН</text>
      {[[71.43, 51.13, 'Астана'], [76.89, 43.24, 'Алматы']].map(([lon, lat, label]) => <g key={label}><circle cx={project(Number(lon), Number(lat))[0]} cy={project(Number(lon), Number(lat))[1]} r="2" className="map-city" /><text x={project(Number(lon), Number(lat))[0] + 8} y={project(Number(lon), Number(lat))[1] + 5} className="plot-label">{label}</text></g>)}
      {points.filter(p => p.longitude >= 45 && p.longitude <= 89 && p.latitude >= 39 && p.latitude <= 57).map(p => <g key={p.index} transform={`translate(${project(p.longitude, p.latitude).join(' ')})`}><circle r="14" className="map-marker-halo" /><circle r="7" className="map-marker" /><title>{p.label}: {p.latitude}, {p.longitude}</title></g>)}
    </svg>
    <div className="map-caption"><span>WGS 84 · офлайн-карта</span><span>Контур: Natural Earth · public domain</span></div>
    {!points.length ? <p className="map-empty">{results.length ? 'Координаты не определены. Адрес не размещён на карте.' : 'Введите адрес — найденное здание появится на карте.'}</p> : <ul className="map-locations">{points.map(p => <li key={p.index}>{onSelect ? <button type="button" onClick={() => onSelect(p.index)}>{p.label}</button> : <strong>{p.label}</strong>}<span>{p.latitude.toFixed(6)}° N, {p.longitude.toFixed(6)}° E</span>{(p.longitude < 45 || p.longitude > 89 || p.latitude < 39 || p.latitude > 57) && <small>За пределами карты Казахстана</small>}</li>)}</ul>}
    <p className="muted">Обзорная карта без улиц. Координаты из демо-каталога синтетические; это не проверенная геопозиция.</p>
  </div>
}

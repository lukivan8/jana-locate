#!/usr/bin/env python3
"""Проверка локальных ссылок публичных руководств и JSON-примеров без зависимостей."""
from pathlib import Path
import json
import re
import sys
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parents[1]
GUIDES = [
    'README.md', 'CONTRIBUTING.md', 'SECURITY.md', 'CODE_OF_CONDUCT.md',
    'THIRD_PARTY_NOTICES.md', 'backend/README.md', 'frontend/README.md',
    'docs/README.md', 'docs/quickstart.md', 'docs/integration.md',
    'docs/deployment.md', 'docs/web.md', 'docs/faq.md', 'docs/architecture.md',
    'docs/roadmap.md', 'docs/verification.md', 'examples/http/README.md',
]


def anchors(text):
    """GitHub-style anchors for ordinary Markdown headings, including Russian."""
    result = set()
    seen = {}
    for line in text.splitlines():
        match = re.match(r'^#{1,6}\s+(.+?)\s*#*$', line)
        if not match:
            continue
        title = match.group(1).lower().replace(' ', '-')
        title = re.sub(r'[^\w\-]', '', title)
        count = seen.get(title, 0)
        seen[title] = count + 1
        result.add(title + (f'-{count}' if count else ''))
    return result


def main():
    errors = []
    links = 0
    for name in GUIDES:
        source = ROOT / name
        text = source.read_text()
        text = re.sub(r'```.*?```', '', text, flags=re.S)
        targets = re.findall(r'\]\(([^\s)]+)(?:\s+"[^"]*")?\)', text)
        targets += re.findall(r'(?:src|href)="([^"]+)"', text)
        for target in targets:
            url = urlsplit(target.strip('<>'))
            if url.scheme or url.netloc:
                continue
            links += 1
            destination = (source.parent / unquote(url.path)).resolve() if url.path else source
            if not destination.is_relative_to(ROOT) or not destination.exists():
                errors.append(f'{name}: нет локальной цели {target}')
                continue
            if url.fragment and destination.suffix == '.md':
                if unquote(url.fragment) not in anchors(destination.read_text()):
                    errors.append(f'{name}: нет заголовка {target}')
    request = json.loads((ROOT / 'examples/http/request.json').read_text())
    response = json.loads((ROOT / 'examples/http/response.fallback.json').read_text())
    spec = json.loads((ROOT / 'backend/api/openapi.json').read_text())
    required = set(spec['components']['schemas']['Result'].get('required', []))
    if len(response['results']) != len(request['records']):
        errors.append('JSON-пример: не совпадает количество записей')
    for original, row in zip(request['records'], response['results']):
        if original['id'] != row['id'] or original['raw_address'] != row['raw_address']:
            errors.append('JSON-пример: изменён исходный ввод')
        missing = required - row.keys()
        if missing:
            errors.append(f'JSON-пример: отсутствуют поля {sorted(missing)}')
        if any(row.get(key) is not None for key in ('coordinates', 'catalog_id', 'catalog_revision')):
            errors.append('JSON-пример: legacy-поля должны быть null')
        if 'libpostal_unavailable' not in row.get('warnings', []):
            errors.append('JSON-пример: не обозначен резервный режим')
    if errors:
        print('\n'.join(errors), file=sys.stderr)
        return 1
    print(f'Проверены {len(GUIDES)} руководств, {links} локальных ссылок и JSON-примеры.')
    return 0


if __name__ == '__main__':
    sys.exit(main())

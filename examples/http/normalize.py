"""Python 3, без внешних пакетов. Запуск: python3 examples/http/normalize.py."""
import json
import os
from pathlib import Path
import sys
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen


def main():
    payload = json.loads(Path(__file__).with_name("request.json").read_text())
    base = os.environ.get("JANA_API_URL", "http://localhost:8080").rstrip("/")
    request = Request(
        base + "/v1/normalize",
        data=json.dumps(payload, ensure_ascii=False).encode("utf-8"),
        headers={
            "Content-Type": "application/json",
            "User-Agent": "JanaLocate/0.1.0 (+https://github.com/lukivan8/jana-locate)",
        }, method="POST",
    )
    try:
        with urlopen(request, timeout=300) as response:
            output = json.load(response)
    except HTTPError as error:
        raise RuntimeError(f"HTTP {error.code}: {error.read().decode('utf-8', errors='replace')}") from error
    if len(output.get("results", [])) != len(payload["records"]):
        raise RuntimeError("API вернул неожиданное количество результатов")
    for source, row in zip(payload["records"], output["results"]):
        if row["id"] != source["id"] or row["raw_address"] != source["raw_address"]:
            raise RuntimeError("API не сохранил исходный ID или адрес")
    print(json.dumps(output, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    try:
        main()
    except (RuntimeError, URLError, TimeoutError, ValueError, OSError, KeyError, TypeError) as error:
        print(f"Ошибка нормализации: {error}", file=sys.stderr)
        sys.exit(1)

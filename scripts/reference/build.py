"""Convenience entrypoint; implementation ships within the standalone backend context."""
import runpy
from pathlib import Path
runpy.run_path(str(Path(__file__).resolve().parents[2]/'backend/tools/build_reference.py'),run_name='__main__')

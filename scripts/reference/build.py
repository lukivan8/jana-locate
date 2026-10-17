"""Convenience entrypoint; implementation ships within the standalone backend context."""
import runpy, sys
from pathlib import Path
tools = Path(__file__).resolve().parents[2]/'backend/tools'
sys.path.insert(0,str(tools))
runpy.run_path(str(tools/'build_reference.py'),run_name='__main__')

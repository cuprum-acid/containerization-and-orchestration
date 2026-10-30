"""Write the public contract to openapi.yaml. Run after every API change:

    python export_openapi.py
"""

from pathlib import Path

import yaml

from app.main import app

spec = app.openapi()
# The version in the spec is what the release pipeline sets, not the code.
spec["info"].pop("version", None)
Path(__file__).with_name("openapi.yaml").write_text(
    yaml.safe_dump(spec, sort_keys=False, allow_unicode=True)
)

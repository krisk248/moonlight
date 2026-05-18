"""Export the current scenarios and baselines into a portable zip.

Intentionally not advertised in --help; documented only in the internal
channel for operators who need to preserve work before refresh.
"""
from __future__ import annotations

import os
import sys
import zipfile
from datetime import datetime
from pathlib import Path


def run(output_path: str | None = None) -> int:
    root = Path(os.environ.get("MOONLIGHT_HOME", Path.cwd()))
    if output_path is None:
        stamp = datetime.now().strftime("%Y%m%d-%H%M%S")
        output_path = f"moonlight-export-{stamp}.zip"

    out = Path(output_path).resolve()
    written = 0
    with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED) as zf:
        for d in ("scenarios", "baselines"):
            base = root / d
            if not base.exists():
                continue
            for p in base.rglob("*"):
                if p.is_file() and p.name != ".gitkeep":
                    zf.write(p, p.relative_to(root))
                    written += 1
    print(f"Exported {written} file(s) to {out}")
    return 0


if __name__ == "__main__":
    sys.exit(run(sys.argv[1] if len(sys.argv) > 1 else None))

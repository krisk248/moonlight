"""Pixel-level image diff with a red overlay highlighting changed regions."""
from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path

import numpy as np
from PIL import Image


@dataclass
class DiffResult:
    ratio: float                  # fraction of pixels that differ
    overlay_path: Path | None     # baseline w/ red highlights, None if identical sizes mismatch handled
    size_mismatch: bool


def _load_rgb(path: Path) -> np.ndarray:
    return np.asarray(Image.open(path).convert("RGB"), dtype=np.int16)


def pixel_diff(baseline: Path, current: Path, overlay_out: Path, tolerance: int = 12) -> DiffResult:
    """Return (ratio, overlay) where ratio is fraction of pixels exceeding `tolerance` channel delta."""
    b = _load_rgb(baseline)
    c = _load_rgb(current)

    if b.shape != c.shape:
        # Resize current to baseline so we can still produce a visual diff.
        c_img = Image.open(current).convert("RGB").resize((b.shape[1], b.shape[0]))
        c = np.asarray(c_img, dtype=np.int16)
        size_mismatch = True
    else:
        size_mismatch = False

    delta = np.abs(b - c).max(axis=-1)        # H x W max channel delta
    changed = delta > tolerance
    ratio = float(changed.mean())

    overlay = b.astype(np.uint8).copy()
    overlay[changed] = [255, 0, 0]            # paint changed pixels red
    overlay_out.parent.mkdir(parents=True, exist_ok=True)
    Image.fromarray(overlay).save(overlay_out)

    return DiffResult(ratio=ratio, overlay_path=overlay_out, size_mismatch=size_mismatch)

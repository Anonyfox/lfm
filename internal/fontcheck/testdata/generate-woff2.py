"""Optional fixture preparation only: fonttools 4.67.0 + brotli 1.2.0."""
from pathlib import Path
from fontTools.ttLib import TTFont

root = Path(__file__).parent
font = TTFont(root / "Go-Regular.ttf", recalcTimestamp=False)
font.flavor = "woff2"
font.save(root / "Go-Regular.woff2")

"""Generates every brand asset for mobile/web from the original logo file.
Run from the repo root:  python3 scripts/generate_assets.py
"""
from PIL import Image
import os

ROOT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..")
BRAND = os.path.normpath(os.path.join(ROOT, "mobile", "assets", "branding"))
WEB = os.path.normpath(os.path.join(ROOT, "mobile", "web"))
os.makedirs(BRAND, exist_ok=True)
os.makedirs(WEB, exist_ok=True)

src = Image.open(os.path.join(ROOT, "..", "inputs", "logo.jpg")).convert("RGB")


def square(im, size):
    w, h = im.size
    m = min(w, h)
    im = im.crop(((w - m) // 2, (h - m) // 2, (w + m) // 2, (h + m) // 2))
    return im.resize((size, size), Image.LANCZOS)


icon = square(src, 1024)
icon.save(os.path.join(BRAND, "icon.png"))
square(src, 512).save(os.path.join(BRAND, "logo.png"))

# Splash: logo centered on the prototype's light background.
spl = Image.new("RGB", (1152, 1152), (255, 255, 255))
lg = square(src, 560)
spl.paste(lg, ((1152 - 560) // 2, (1152 - 560) // 2))
spl.save(os.path.join(BRAND, "splash.png"))

# Web favicons.
icon.save(os.path.join(WEB, "favicon.png"))
icon.resize((32, 32), Image.LANCZOS).save(
    os.path.join(WEB, "favicon.ico"), sizes=[(16, 16), (32, 32)]
)
print("assets written:", sorted(os.listdir(BRAND)))
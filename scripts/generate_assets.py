"""Generates every brand asset for mobile/web from the original logo file.
Run from the repo root:  python3 scripts/generate_assets.py

Single source of truth: assets/logo.png (the delivered Nexus logo).
Outputs:
  mobile/assets/branding/            icon.png 1024, logo.png 512, splash.png 1152
  mobile/android/.../res/            launcher splash per density (light + night)
  mobile/web/splash/img/             light/dark 1x-4x splash
  mobile/ios/.../LaunchImage.imageset  1x/2x/3x, light + dark
  mobile/web/                        favicon.png / favicon.ico
Backgrounds: Android/branding splash artwork is transparent (the platform
window background supplies the color); web/iOS splash bakes SPLASH_BG.
"""
from PIL import Image
import os

ROOT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..")
BRAND = os.path.normpath(os.path.join(ROOT, "mobile", "assets", "branding"))
WEB = os.path.normpath(os.path.join(ROOT, "mobile", "web"))
ANDROID_RES = os.path.normpath(
    os.path.join(ROOT, "mobile", "android", "app", "src", "main", "res")
)
IOS_LAUNCH = os.path.normpath(
    os.path.join(
        ROOT, "mobile", "ios", "Runner", "Assets.xcassets", "LaunchImage.imageset"
    )
)
for d in (BRAND, WEB, os.path.join(WEB, "splash", "img")):
    os.makedirs(d, exist_ok=True)

# The delivered logo (RGBA, transparent padding around the mark).
src = Image.open(os.path.join(ROOT, "assets", "logo.png")).convert("RGBA")

# Slate-900-ish window background, baked into web/iOS splash artwork.
SPLASH_BG = (11, 18, 32)
# The source image is drawn at this fraction of the splash canvas; the mark
# itself then occupies ~39% of the canvas (measured from the shipped assets).
SPLASH_DRAW_FRACTION = 0.46

ANDROID_DENSITIES = {"mdpi": 288, "hdpi": 432, "xhdpi": 576, "xxhdpi": 864, "xxxhdpi": 1152}
WEB_SCALES = {"1x": 288, "2x": 576, "3x": 864, "4x": 1152}
IOS_SCALES = {"": 288, "@2x": 576, "@3x": 864}


def square(im, size):
    w, h = im.size
    m = min(w, h)
    im = im.crop(((w - m) // 2, (h - m) // 2, (w + m) // 2, (h + m) // 2))
    return im.resize((size, size), Image.LANCZOS)


def splash(size, bg):
    """Logo centered on a size x size canvas; bg=None keeps the canvas
    transparent (Android window background supplies the color)."""
    if bg is None:
        canvas = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    else:
        canvas = Image.new("RGB", (size, size), bg)
    side = round(size * SPLASH_DRAW_FRACTION)
    logo = square(src, side)
    canvas.paste(logo, ((size - side) // 2, (size - side) // 2), logo)
    return canvas


icon = square(src, 1024)
icon.save(os.path.join(BRAND, "icon.png"))
square(src, 512).save(os.path.join(BRAND, "logo.png"))

# Splash master: transparent artwork; the launcher background shows through.
splash(1152, None).save(os.path.join(BRAND, "splash.png"))

# Android launch splash per density, light + night (same transparent artwork).
for name, size in ANDROID_DENSITIES.items():
    for prefix in ("drawable", "drawable-night"):
        d = os.path.join(ANDROID_RES, f"{prefix}-{name}")
        os.makedirs(d, exist_ok=True)
        splash(size, None).save(os.path.join(d, "splash.png"))

# Web splash, light + dark (background baked, as shipped: both dark).
for scale, size in WEB_SCALES.items():
    splash(size, SPLASH_BG).save(os.path.join(WEB, "splash", "img", f"light-{scale}.png"))
    splash(size, SPLASH_BG).save(os.path.join(WEB, "splash", "img", f"dark-{scale}.png"))

# iOS launch image (background baked; LaunchScreen.storyboard centers it).
for suffix, size in IOS_SCALES.items():
    img = splash(size, SPLASH_BG)
    img.save(os.path.join(IOS_LAUNCH, f"LaunchImage{suffix}.png"))
    img.save(os.path.join(IOS_LAUNCH, f"LaunchImageDark{suffix}.png"))

# Web favicons.
icon.save(os.path.join(WEB, "favicon.png"))
icon.resize((32, 32), Image.LANCZOS).save(
    os.path.join(WEB, "favicon.ico"), sizes=[(16, 16), (32, 32)]
)
print("assets written:", sorted(os.listdir(BRAND)))

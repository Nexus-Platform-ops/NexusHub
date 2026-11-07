# Asset integration evidence

- Source brand input: blue diamond/propeller logo (4096x4096 jpg).
- Generated branding assets in mobile/assets/branding/: icon.png, logo.png, splash.png
- Web favicon: mobile/web/favicon.png, mobile/web/favicon.ico.
- Generator: scripts/generate_assets.py (Pillow; derives icon/logo/splash sizes and favicon from the brand mark).
- Wiring: mobile/pubspec.yaml declares assets/branding/ and configures flutter_launcher_icons (adaptive icon) and flutter_native_splash (brand splash on #0B1220 dark / #FFF8EC light). Theme colors: amber #F59E0B (social), rose #F43F5E (dating), emerald #10B981 (market) in mobile/lib/theme.dart.
- Status: files exist on disk and are referenced by pubspec; on-device render check is BLOCKED (no emulator in sandbox).

# Image assets

| File | Used by | Size | Status |
|------|---------|------|--------|
| `banner.png` | README header | 1280×320 | ✅ generated |
| `og.png` | Social preview (GitHub + landing page OG/Twitter card) | 1280×640 | ✅ generated |
| `favicon.png` | Landing page favicon | 256×256 | ✅ generated |
| `telegram-alert.png` | README + landing-page screenshot | ~420-800px wide | ⬜ **you add this** |

The generated images are vector-sourced from `_src/*.svg`. Edit the SVG and re-run the
render command below to regenerate.

## The one image to add: `telegram-alert.png`

- **Place it here:** `docs/assets/telegram-alert.png` (exact name, lowercase).
- **What to capture:** a real OxiWatch message in Telegram, ideally one successful-login
  alert and/or the daily failed-attempt report in the same shot.
- **Size:** anything from ~420px to ~800px wide; portrait phone-screenshot crop looks best.
- **Privacy:** blur or redact real server names, usernames, and your own IPs before publishing.
- It's referenced by both `README.md` and `docs/index.html`. The landing page hides the
  screenshot section automatically until the file exists, so nothing breaks in the meantime.

## Regenerating the brand images

```bash
cd docs/assets
rsvg-convert -w 1280 -h 320 _src/banner.svg  -o banner.png
rsvg-convert -w 1280 -h 640 _src/og.svg      -o og.png
rsvg-convert -w 256  -h 256 _src/favicon.svg -o favicon.png
```

## Don't forget
After everything's pushed, upload `og.png` under GitHub repo
**Settings → General → Social preview** so shared links show the card.

# Synthetic codec fixtures

All pixels are generated, with no real photos or personal metadata. The source
is a 48 × 32 sRGB raster with red RGB (214, 48, 33) above blue (28, 109, 220).
Apple `sips -s format heic -s formatOptions 90`, `sips -s format tiff`, and
`sips -s format gif` encoded the respective single image fixtures. Image I/O
wrote two copies of the raster into the two-page TIFF. The 24-bit BMP stores
the same rows bottom-up. The ICO stores two PNG representations: opaque red
16 × 16, and 32 × 32 with half-transparent blue on the left and transparent
pixels on the right.

For the second format expansion, the same red/blue raster was encoded with
`sips -s format jp2` and `sips -s format psd` for the JP2 and composite PSD.
`opj_compress -n 4` encoded the raw J2K codestream. Libvips `jxlsave --lossless`
encoded the JPEG XL, and `--page-height 32` saved a two-frame version from a
vertical stack. The transparency fixtures use a 32 × 24 raster with
half-transparent cyan (15, 190, 220, 128) on the left and fully transparent
pixels on the right, encoded with `sips` and `jxlsave`.

Sequence fixtures are also synthetic. `timed.gif` uses three colored frames,
partial rectangles, restore-to-previous disposal, and 70/130/220 ms delays.
`timed.jxl` comes from that GIF via libvips `jxlsave --lossless` with all pages;
the older `animated.jxl` is an untimed two-image collection. The APNG fixtures
use PNG chunks with different delay, blend and disposal values. `poster.apng`
and `alpha-timed.apng` include a separate yellow default image outside the
animation. `timed.avis` uses FFmpeg's 96 × 96 `testsrc2` at 4 fps, encoded by
SVT-AV1. macOS Image I/O wrote `timed.heics` and `different-pages.tiff` from
solid colors; the TIFF includes three page sizes and a translucent middle page.

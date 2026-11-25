package imaging

import "github.com/davidbyttow/govips/v2/vips"

func imageFromRGBA(pixels []byte, info Info) (*vips.ImageRef, error) {
	im, err := vips.NewImageFromMemory(pixels, info.Width, info.Height, 4, vips.BandFormatUchar, vips.InterpretationSRGB)
	if err != nil {
		return nil, err
	}
	if !info.Alpha {
		if err := im.ExtractBand(0, 3); err != nil {
			im.Close()
			return nil, err
		}
	}
	return im, nil
}

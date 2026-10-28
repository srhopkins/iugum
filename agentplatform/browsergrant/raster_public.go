package browsergrant

import (
	"bytes"
	"errors"
	"image"
	"image/png"
)

// ValidatePNG is the shared pure raster boundary for tool/model image consumers.
// It grants no capture/input authority and preserves the original encoded bytes.
func ValidatePNG(data []byte) (image.Config, error) {
	if err := strictPNG(data); err != nil {
		return image.Config{}, err
	}
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 8192 || config.Height > 8192 || int64(config.Width)*int64(config.Height) > MaxRasterPixels {
		return image.Config{}, errors.New("unsafe PNG dimensions")
	}
	if _, err = png.Decode(bytes.NewReader(data)); err != nil {
		return image.Config{}, errors.New("invalid PNG raster")
	}
	return config, nil
}

package ipa

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"io"
	"os"
	"path"
	"strings"

	"golang.org/x/image/draw"
	"howett.net/plist"
)

const maxIconBytes = 4 << 20

// DecodeCustomIcon validates an optional PNG/JPEG data URL before decoding it.
func DecodeCustomIcon(data string) (image.Image, error) {
	if len(data) > base64.StdEncoding.EncodedLen(maxIconBytes)+32 {
		return nil, fmt.Errorf("custom icon must be at most 4 MB")
	}
	prefix, encoded, ok := strings.Cut(data, ",")
	if !ok || (prefix != "data:image/png;base64" && prefix != "data:image/jpeg;base64") {
		return nil, fmt.Errorf("custom icon must be a PNG or JPEG image")
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("invalid custom icon: %w", err)
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || (format != "png" && format != "jpeg") {
		return nil, fmt.Errorf("custom icon must be a PNG or JPEG image")
	}
	if config.Width < 1 || config.Height < 1 || config.Width > 4096 || config.Height > 4096 {
		return nil, fmt.Errorf("custom icon dimensions must be between 1 and 4096 pixels")
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	return img, err
}

// iconImage center-crops to the target aspect ratio and flattens transparency
// onto black, as home screen icons require an opaque background.
func iconImage(src image.Image, width, height int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.Black), image.Point{}, draw.Src)
	crop := src.Bounds()
	if crop.Dx()*height > crop.Dy()*width {
		w := max(1, crop.Dy()*width/height)
		crop.Min.X += (crop.Dx() - w) / 2
		crop.Max.X = crop.Min.X + w
	} else {
		h := max(1, crop.Dx()*height/width)
		crop.Min.Y += (crop.Dy() - h) / 2
		crop.Max.Y = crop.Min.Y + h
	}
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, crop, draw.Over, nil)
	return dst
}

// WithCustomIcon creates a temporary IPA with its main app icon replaced.
// Signing consumes this copy; the original IPA is left intact for future updates.
func WithCustomIcon(ipaPath, data string) (_ string, err error) {
	img, err := DecodeCustomIcon(data)
	if err != nil {
		return "", err
	}
	r, err := zip.OpenReader(ipaPath)
	if err != nil {
		return "", err
	}
	defer func() { _ = r.Close() }()
	var infoFile *zip.File
	for _, f := range r.File {
		if strings.HasPrefix(f.Name, "Payload/") && strings.Count(f.Name, "/") == 2 && strings.HasSuffix(f.Name, ".app/Info.plist") {
			if infoFile != nil {
				return "", fmt.Errorf("IPA contains multiple main apps")
			}
			infoFile = f
		}
	}
	if infoFile == nil {
		return "", ErrInfoPlistNotFound
	}
	raw, err := readIconEntry(infoFile, 4<<20)
	if err != nil {
		return "", err
	}
	var info map[string]interface{}
	if _, err = plist.Unmarshal(raw, &info); err != nil {
		return "", err
	}
	root := path.Dir(infoFile.Name) + "/"
	replacements := map[string][]byte{}
	if icons, ok := info["CFBundleIcons"].(map[string]interface{}); ok {
		if primary, ok := icons["CFBundlePrimaryIcon"].(string); ok {
			var catalog *zip.File
			for _, f := range r.File {
				if f.Name == root+"Assets.car" {
					catalog = f
				}
			}
			if catalog == nil {
				return "", fmt.Errorf("tvOS icon asset catalog is missing")
			}
			car, err := readIconEntry(catalog, 256<<20)
			if err != nil {
				return "", err
			}
			car, err = replaceCatalogIcon(car, primary, img)
			if err != nil {
				return "", err
			}
			replacements[catalog.Name] = car
		}
	}
	if len(replacements) == 0 {
		if platforms, ok := info["CFBundleSupportedPlatforms"].([]interface{}); ok {
			for _, platform := range platforms {
				if platform == "AppleTVOS" {
					return "", fmt.Errorf("tvOS IPA has no supported primary icon asset")
				}
			}
		}
		// iPhone/iPad use explicit icon files, avoiding a dependency on their catalog.
		for _, size := range []int{120, 152, 167, 180} {
			var b bytes.Buffer
			if err := png.Encode(&b, iconImage(img, size, size)); err != nil {
				return "", err
			}
			replacements[fmt.Sprintf("%sATVCustomIcon%d.png", root, size)] = b.Bytes()
		}
		primary := map[string]interface{}{"CFBundleIconFiles": []string{"ATVCustomIcon120", "ATVCustomIcon180"}}
		info["CFBundleIcons"] = map[string]interface{}{"CFBundlePrimaryIcon": primary}
		info["CFBundleIcons~ipad"] = map[string]interface{}{"CFBundlePrimaryIcon": map[string]interface{}{"CFBundleIconFiles": []string{"ATVCustomIcon152", "ATVCustomIcon167"}}}
		delete(info, "CFBundleIconName")
		delete(info, "CFBundleIconFile")
		info["CFBundleIconFiles"] = []string{"ATVCustomIcon120", "ATVCustomIcon180"}
		raw, err = plist.Marshal(info, plist.BinaryFormat)
		if err != nil {
			return "", err
		}
		replacements[infoFile.Name] = raw
	}
	out, err := os.CreateTemp(path.Dir(ipaPath), "custom-icon-*.ipa")
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			_ = out.Close()
			_ = os.Remove(out.Name())
		}
	}()
	w := zip.NewWriter(out)
	for _, f := range r.File {
		if _, ok := replacements[f.Name]; ok {
			continue
		}
		if err = w.Copy(f); err != nil {
			_ = w.Close()
			return "", err
		}
	}
	for name, content := range replacements {
		var dest io.Writer
		dest, err = w.Create(name)
		if err != nil {
			_ = w.Close()
			return "", err
		}
		if _, err = dest.Write(content); err != nil {
			_ = w.Close()
			return "", err
		}
	}
	if err = w.Close(); err != nil {
		return "", err
	}
	if err = out.Close(); err != nil {
		return "", err
	}
	return out.Name(), nil
}

func readIconEntry(f *zip.File, limit int64) ([]byte, error) {
	if f.UncompressedSize64 > uint64(limit) {
		return nil, fmt.Errorf("icon resource %s is too large", f.Name)
	}
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("icon resource %s is too large", f.Name)
	}
	return b, nil
}

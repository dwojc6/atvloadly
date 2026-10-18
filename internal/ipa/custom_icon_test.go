package ipa

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/iineva/bom/pkg/asset"
	"github.com/iineva/bom/pkg/bom"
	"howett.net/plist"
)

func testCustomIcon(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.RGBA{0, 255, 0, 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}

func TestCustomIconValidation(t *testing.T) {
	for _, data := range []string{"/etc/passwd", "data:image/svg+xml;base64,AAAA", "data:image/png;base64,broken", "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("not an image"))} {
		if _, err := DecodeCustomIcon(data); err == nil {
			t.Fatalf("accepted %q", data)
		}
	}
	if _, err := DecodeCustomIcon(testCustomIcon(t)); err != nil {
		t.Fatal(err)
	}
}

func TestCustomIconCatalog(t *testing.T) {
	raw, err := os.ReadFile("testdata/custom-icon/Assets.car")
	if err != nil {
		t.Fatal(err)
	}
	img, err := DecodeCustomIcon(testCustomIcon(t))
	if err != nil {
		t.Fatal(err)
	}
	updated, err := replaceCatalogIcon(raw, "App Icon", img)
	if err != nil {
		t.Fatal(err)
	}
	a, err := asset.NewWithReadSeeker(bytes.NewReader(updated))
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := a.ImageCandidates("icon")
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) == 0 {
		t.Fatal("icon renditions disappeared")
	}
	for _, c := range candidates {
		t.Logf("%s %s %dx%d", c.Name, c.RenditionName, c.Width, c.Height)
		got := color.NRGBAModel.Convert(c.Image.At(c.Width/2, c.Height/2)).(color.NRGBA)
		if c.Name == "App Icon/Front/Content" && got.A != 0 {
			t.Fatal("foreground was not cleared")
		}
		if isFlattenedImage(c.RenditionName) || c.Name == "App Icon/Background/Content" {
			if got.G != 255 || got.R != 0 || got.B != 0 {
				t.Fatalf("icon still has original pixels: %+v", got)
			}
		}
	}
	originalOther := catalogImageData(t, raw, "Unrelated")
	updatedOther := catalogImageData(t, updated, "Unrelated")
	if len(originalOther) == 0 || !bytes.Equal(originalOther, updatedOther) {
		t.Fatal("unrelated catalog image changed")
	}
	// Apple's assetutil can independently inspect this result during manual QA.
	if output := os.Getenv("ATV_ICON_TEST_OUTPUT"); output != "" {
		if err := os.WriteFile(output, updated, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := replaceCatalogIcon(raw, "Missing", img); err == nil {
		t.Fatal("accepted missing icon")
	}
}

func TestWithCustomIcon(t *testing.T) {
	for _, tv := range []bool{false, true} {
		t.Run(map[bool]string{false: "iOS", true: "tvOS"}[tv], func(t *testing.T) {
			original := filepath.Join(t.TempDir(), "original.ipa")
			info := map[string]interface{}{"CFBundleIdentifier": "com.example.app", "CFBundleName": "Example"}
			entries := map[string][]byte{"Payload/Main.app/Main": []byte("executable"), "Payload/Main.app/PlugIns/Widget.appex/Info.plist": []byte("unchanged")}
			if tv {
				info["CFBundleIcons"] = map[string]interface{}{"CFBundlePrimaryIcon": "App Icon"}
				car, err := os.ReadFile("testdata/custom-icon/Assets.car")
				if err != nil {
					t.Fatal(err)
				}
				entries["Payload/Main.app/Assets.car"] = car
			}
			raw, err := plist.Marshal(info, plist.BinaryFormat)
			if err != nil {
				t.Fatal(err)
			}
			entries["Payload/Main.app/Info.plist"] = raw
			var archive bytes.Buffer
			w := zip.NewWriter(&archive)
			for name, data := range entries {
				f, e := w.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, ExternalAttrs: 0755 << 16})
				if e != nil {
					t.Fatal(e)
				}
				if _, e = f.Write(data); e != nil {
					t.Fatal(e)
				}
			}
			if err = w.Close(); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(original, archive.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			patched, err := WithCustomIcon(original, testCustomIcon(t))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = os.Remove(patched) }()
			unchanged, err := os.ReadFile(original)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(unchanged, archive.Bytes()) {
				t.Fatal("original IPA was modified")
			}
			r, err := zip.OpenReader(patched)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = r.Close() }()
			found := false
			for _, f := range r.File {
				if f.Name == "Payload/Main.app/Main" || f.Name == "Payload/Main.app/PlugIns/Widget.appex/Info.plist" {
					reader, e := f.Open()
					if e != nil {
						t.Fatal(e)
					}
					content, e := io.ReadAll(reader)
					_ = reader.Close()
					if e != nil {
						t.Fatal(e)
					}
					if !bytes.Equal(content, entries[f.Name]) {
						t.Fatalf("changed unrelated entry %s", f.Name)
					}
				}
				if f.Name == "Payload/Main.app/ATVCustomIcon180.png" {
					found = true
				}
			}
			if !tv && !found {
				t.Fatal("replacement PNG not added")
			}
		})
	}
}

func catalogImageData(t *testing.T, data []byte, name string) []byte {
	t.Helper()
	b := bom.New(bytes.NewReader(data))
	if err := b.Parse(); err != nil {
		t.Fatal(err)
	}
	a := asset.New(b)
	facets, err := a.FacetKeys()
	if err != nil {
		t.Fatal(err)
	}
	id := uint16(facets[name][asset.RenditionAttributeType(17)])
	keys, err := a.KeyFormat()
	if err != nil {
		t.Fatal(err)
	}
	var result []byte
	err = b.ReadTree("RENDITIONS", func(k, d io.Reader) error {
		var value uint16
		for _, token := range keys.RenditionKeyTokens {
			if err := binary.Read(k, binary.LittleEndian, &value); err != nil {
				return err
			}
			if token == asset.RenditionAttributeType(17) && value == id {
				raw, err := io.ReadAll(d)
				if err != nil {
					return err
				}
				result = append(result, raw...)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

package ipa

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"io"
	"strings"

	"github.com/iineva/bom/pkg/asset"
	"github.com/iineva/bom/pkg/bom"
)

// replaceCatalogIcon preserves catalog keys and layer geometry, replacing the
// primary icon's back and flattened images, clearing its foreground layers.
// Unrelated images, top shelf artwork and alternate icons are left untouched.
func replaceCatalogIcon(data []byte, name string, img image.Image) ([]byte, error) {
	if len(data) < 512 || string(data[:8]) != "BOMStore" {
		return nil, fmt.Errorf("invalid icon asset catalog")
	}
	be, le := binary.BigEndian, binary.LittleEndian
	index := int(be.Uint32(data[16:20]))
	indexLen := int(be.Uint32(data[20:24]))
	if index < 512 || indexLen < 4 || index > len(data)-indexLen {
		return nil, fmt.Errorf("invalid catalog block table")
	}
	count := int(be.Uint32(data[index : index+4]))
	if count > (indexLen-4)/8 {
		return nil, fmt.Errorf("invalid catalog block count")
	}
	// Validate offsets before passing the catalog to the existing BOM reader.
	blocks := map[int64][]int{}
	for i := 0; i < count; i++ {
		p := index + 4 + i*8
		start, size := int(be.Uint32(data[p:p+4])), int(be.Uint32(data[p+4:p+8]))
		if start > len(data)-size {
			return nil, fmt.Errorf("invalid catalog block bounds")
		}
		if size > 0 {
			blocks[int64(start)] = append(blocks[int64(start)], i)
		}
	}
	source := bytes.NewReader(data)
	b := bom.New(source)
	if err := b.Parse(); err != nil {
		return nil, err
	}
	a := asset.New(b)
	facets, err := a.FacetKeys()
	if err != nil {
		return nil, err
	}
	keyFormat, err := a.KeyFormat()
	if err != nil {
		return nil, err
	}
	idPosition := -1
	for i, token := range keyFormat.RenditionKeyTokens {
		if token == asset.RenditionAttributeType(17) {
			idPosition = i
		}
	}
	if idPosition < 0 {
		return nil, fmt.Errorf("catalog has no rendition identifiers")
	}
	names := map[uint16]string{}
	for facet, attrs := range facets {
		facet = strings.TrimRight(facet, "\x00")
		if facet == name || strings.HasPrefix(facet, name+"/") {
			names[uint16(attrs[asset.RenditionAttributeType(17)])] = facet
		}
	}
	// Stack metadata lists layers from back to front. Use its identifier,
	// rather than assuming the author named the bottom layer "Back".
	backLayers := map[uint16]bool{}
	err = b.ReadTree("RENDITIONS", func(k, d io.Reader) error {
		key, err := io.ReadAll(k)
		if err != nil {
			return err
		}
		if len(key) < 2*(idPosition+1) {
			return fmt.Errorf("invalid rendition key")
		}
		if names[le.Uint16(key[idPosition*2:])] != name {
			return nil
		}
		raw, err := io.ReadAll(d)
		if err != nil {
			return err
		}
		if len(raw) < 184 || string(raw[24:28]) != "ATAD" {
			return nil
		}
		end := 184 + int(le.Uint32(raw[168:172]))
		if end > len(raw) {
			return fmt.Errorf("invalid stack metadata")
		}
		for pos := 184; pos+8 <= end; {
			tag, size := le.Uint32(raw[pos:]), int(le.Uint32(raw[pos+4:]))
			pos += 8
			if size > end-pos {
				return fmt.Errorf("invalid stack metadata length")
			}
			if tag == 1012 && size >= 40 {
				value := raw[pos : pos+size]
				keyLength := int(le.Uint32(value[36:40]))
				if keyLength < 4 || keyLength > len(value)-36 {
					return fmt.Errorf("invalid layer key")
				}
				for offset := 40; offset+4 <= 36+keyLength; offset += 4 {
					if le.Uint16(value[offset:]) == 17 {
						backLayers[le.Uint16(value[offset+2:])] = true
					}
				}
			}
			pos += size
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	changes := map[int][]byte{}
	err = b.ReadTree("RENDITIONS", func(k, d io.Reader) error {
		key, err := io.ReadAll(k)
		if err != nil {
			return err
		}
		if len(key) < 2*(idPosition+1) {
			return fmt.Errorf("invalid rendition key")
		}
		id := le.Uint16(key[idPosition*2:])
		facet, ok := names[id]
		if !ok {
			return nil
		}
		raw, err := io.ReadAll(d)
		if err != nil {
			return err
		}
		if len(raw) < 184 || string(raw[:4]) != "ISTC" {
			return fmt.Errorf("invalid icon rendition")
		}
		format := string(raw[24:28])
		// DATA stores the layer composition, rather than pixels.
		if format == "ATAD" || format == "\x00\x00\x00\x00" {
			return nil
		}
		switch format {
		case "BGRA", "ARGB", " 8AG", "5BGR", "WBGR", "61AG", "GEPJ", "FIEH":
		default:
			return fmt.Errorf("unsupported icon pixel format %q", format)
		}
		width, height := int(le.Uint32(raw[12:16])), int(le.Uint32(raw[16:20]))
		tlv := int(le.Uint32(raw[168:172]))
		headerLen := 184 + tlv
		if width < 1 || height < 1 || width > 4096 || height > 4096 || headerLen > len(raw) {
			return fmt.Errorf("invalid icon rendition dimensions")
		}
		pixels := make([]byte, width*height*4)
		if strings.Contains(facet, "/Content") && len(backLayers) == 0 {
			return fmt.Errorf("icon stack has no supported background layer")
		}
		if !strings.Contains(facet, "/Content") || backLayers[id] {
			resized := iconImage(img, width, height)
			for p := 0; p < len(pixels); p += 4 {
				pixels[p] = resized.Pix[p+2]
				pixels[p+1] = resized.Pix[p+1]
				pixels[p+2] = resized.Pix[p]
				pixels[p+3] = resized.Pix[p+3]
			}
		}
		replacement := append([]byte(nil), raw[:headerLen]...)
		// CSI pixel format tags are stored in reverse order (ARGB -> BGRA).
		copy(replacement[24:28], "BGRA")
		le.PutUint32(replacement[8:12], le.Uint32(replacement[8:12])&^0xf8)
		// The original may use grayscale pixels or padded scanlines. This
		// replacement is tightly packed BGRA, so update CoreUI's row stride
		// alongside the pixel format. A stale stride makes focused layer
		// images fail to materialize even when the flattened icon works.
		for pos := 184; pos+8 <= headerLen; {
			tag, size := le.Uint32(replacement[pos:]), int(le.Uint32(replacement[pos+4:]))
			pos += 8
			if size > headerLen-pos {
				return fmt.Errorf("invalid icon bitmap metadata")
			}
			if tag == 1007 {
				if size != 4 {
					return fmt.Errorf("invalid icon row stride metadata")
				}
				le.PutUint32(replacement[pos:pos+4], uint32(width*4))
			}
			pos += size
		}
		le.PutUint32(replacement[28:32], 1) // sRGB
		le.PutUint32(replacement[180:184], uint32(16+len(pixels)))
		pixelHeader := make([]byte, 16)
		copy(pixelHeader, "MLEC")
		le.PutUint32(pixelHeader[4:8], 0)
		le.PutUint32(pixelHeader[8:12], 0) // uncompressed
		le.PutUint32(pixelHeader[12:16], uint32(len(pixels)))
		replacement = append(replacement, pixelHeader...)
		replacement = append(replacement, pixels...)
		end, err := source.Seek(0, io.SeekCurrent)
		if err != nil {
			return err
		}
		for _, i := range blocks[end-int64(len(raw))] {
			changes[i] = replacement
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("replace app icon: %w", err)
	}
	if len(changes) == 0 {
		return nil, fmt.Errorf("primary app icon %q has no replaceable images", name)
	}
	// Keep block IDs and tree references stable. Append replacements and relocate
	// the block table to EOF, updating only its pointers and the BOM header.
	out := append([]byte(nil), data...)
	table := append([]byte(nil), data[index:index+indexLen]...)
	for i, replacement := range changes {
		for len(out)%16 != 0 {
			out = append(out, 0)
		}
		p := 4 + i*8
		be.PutUint32(table[p:p+4], uint32(len(out)))
		be.PutUint32(table[p+4:p+8], uint32(len(replacement)))
		out = append(out, replacement...)
	}
	be.PutUint32(out[16:20], uint32(len(out)))
	out = append(out, table...)
	return out, nil
}

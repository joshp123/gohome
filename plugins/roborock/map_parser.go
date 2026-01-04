package roborock

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
)

const (
	mapBlockImage     = 2
	mapBlockCarpetMap = 17
	mapBlockRobot     = 8
)

type mapImage struct {
	png    []byte
	width  int
	height int
}

func parseMapImage(raw []byte) (mapImage, error) {
	data := raw
	if len(raw) >= 2 && raw[0] == 0x1f && raw[1] == 0x8b {
		decompressed, err := gzipDecompress(raw)
		if err != nil {
			return mapImage{}, err
		}
		data = decompressed
	}

	imageBlock, robotPos, carpetMap, err := extractMapImageBlock(data)
	if err != nil {
		return mapImage{}, err
	}
	if imageBlock == nil {
		return mapImage{}, fmt.Errorf("map image block not found")
	}

	pngBytes, width, height, err := renderMapPNG(*imageBlock, robotPos, carpetMap)
	if err != nil {
		return mapImage{}, err
	}
	return mapImage{png: pngBytes, width: width, height: height}, nil
}

type mapImageBlock struct {
	left   int
	top    int
	width  int
	height int
	data   []byte
}

type mapPoint struct {
	x int
	y int
}

func extractMapImageBlock(raw []byte) (*mapImageBlock, *mapPoint, map[int]struct{}, error) {
	if len(raw) < 4 {
		return nil, nil, nil, fmt.Errorf("map payload too short")
	}
	mapHeaderLen := int(int16le(raw, 0x02))
	if mapHeaderLen <= 0 || mapHeaderLen >= len(raw) {
		return nil, nil, nil, fmt.Errorf("invalid map header length")
	}

	var imageBlock *mapImageBlock
	var robotPos *mapPoint
	carpetMap := make(map[int]struct{})
	blockStart := mapHeaderLen
	for blockStart < len(raw) {
		if blockStart+8 > len(raw) {
			break
		}
		blockHeaderLen := int(int16le(raw, blockStart+0x02))
		if blockHeaderLen == 0 || blockStart+blockHeaderLen > len(raw) {
			break
		}
		header := raw[blockStart : blockStart+blockHeaderLen]
		blockType := int(int16le(header, 0x00))
		blockDataLen := int(int32le(header, 0x04))
		blockDataStart := blockStart + blockHeaderLen
		if blockDataStart+blockDataLen > len(raw) || blockDataLen < 0 {
			break
		}
		data := raw[blockDataStart : blockDataStart+blockDataLen]

		switch blockType {
		case mapBlockImage:
			if blockHeaderLen < 16 {
				break
			}
			top := int(int32le(header, blockHeaderLen-16))
			left := int(int32le(header, blockHeaderLen-12))
			height := int(int32le(header, blockHeaderLen-8))
			width := int(int32le(header, blockHeaderLen-4))
			if width > 0 && height > 0 && len(data) >= width*height {
				imageBlock = &mapImageBlock{
					left:   left,
					top:    top,
					width:  width,
					height: height,
					data:   data,
				}
			}
		case mapBlockCarpetMap:
			for i, v := range data {
				if v != 0 {
					carpetMap[i] = struct{}{}
				}
			}
		case mapBlockRobot:
			if blockDataLen >= 8 {
				x := int(int32le(data, 0x00))
				y := int(int32le(data, 0x04))
				robotPos = &mapPoint{x: x, y: y}
			}
		}

		blockStart = blockStart + blockDataLen + int(int8(raw[blockStart+2]))
	}

	return imageBlock, robotPos, carpetMap, nil
}

func renderMapPNG(block mapImageBlock, robot *mapPoint, carpetMap map[int]struct{}) ([]byte, int, int, error) {
	width := block.width
	height := block.height
	if width == 0 || height == 0 {
		return nil, 0, 0, fmt.Errorf("invalid map dimensions")
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for imgY := 0; imgY < height; imgY++ {
		offset := width * imgY
		for imgX := 0; imgX < width; imgX++ {
			idx := imgX + offset
			pixelType := block.data[idx]
			x := imgX
			y := height - imgY - 1
			var c color.RGBA
			if _, ok := carpetMap[idx]; ok && (x+y)%2 == 1 {
				c = colorCarpet()
			} else if pixelType == 0x00 {
				c = colorOutside()
			} else if pixelType == 0x01 {
				c = colorWall()
			} else if pixelType == 0xFF {
				c = colorInside()
			} else if pixelType == 0x07 {
				c = colorScan()
			} else {
				obstacle := pixelType & 0x07
				switch obstacle {
				case 0:
					c = colorGreyWall()
				case 1:
					c = colorWallV2()
				case 7:
					room := int(pixelType) >> 3
					c = colorRoom(room)
				default:
					c = colorUnknown()
				}
			}
			img.SetRGBA(x, y, c)
		}
	}

	if robot != nil {
		rx := int(math.Round(float64(robot.x)/50.0)) - block.left
		ry := int(math.Round(float64(robot.y)/50.0)) - block.top
		drawDot(img, rx, height-ry-1, colorRobot())
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, 0, 0, err
	}
	return buf.Bytes(), width, height, nil
}

func gzipDecompress(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func int8(b byte) int {
	return int(int8(b))
}

func int16le(data []byte, offset int) uint16 {
	return uint16(data[offset]) | uint16(data[offset+1])<<8
}

func int32le(data []byte, offset int) uint32 {
	return uint32(data[offset]) | uint32(data[offset+1])<<8 | uint32(data[offset+2])<<16 | uint32(data[offset+3])<<24
}

func drawDot(img *image.RGBA, x, y int, c color.RGBA) {
	if x < 0 || y < 0 || x >= img.Bounds().Dx() || y >= img.Bounds().Dy() {
		return
	}
	for dy := -2; dy <= 2; dy++ {
		for dx := -2; dx <= 2; dx++ {
			xx := x + dx
			yy := y + dy
			if xx < 0 || yy < 0 || xx >= img.Bounds().Dx() || yy >= img.Bounds().Dy() {
				continue
			}
			if dx*dx+dy*dy <= 4 {
				img.SetRGBA(xx, yy, c)
			}
		}
	}
}

func colorOutside() color.RGBA { return color.RGBA{0, 0, 0, 0} }
func colorWall() color.RGBA    { return color.RGBA{40, 40, 40, 255} }
func colorWallV2() color.RGBA  { return color.RGBA{60, 60, 60, 255} }
func colorInside() color.RGBA  { return color.RGBA{230, 230, 230, 255} }
func colorScan() color.RGBA    { return color.RGBA{200, 220, 255, 255} }
func colorGreyWall() color.RGBA {
	return color.RGBA{90, 90, 90, 255}
}
func colorUnknown() color.RGBA { return color.RGBA{180, 80, 180, 255} }
func colorCarpet() color.RGBA  { return color.RGBA{220, 160, 90, 255} }
func colorRobot() color.RGBA   { return color.RGBA{240, 70, 70, 255} }

func colorRoom(room int) color.RGBA {
	h := float64((room * 47) % 360)
	s := 0.45
	v := 0.9
	r, g, b := hsvToRGB(h, s, v)
	return color.RGBA{r, g, b, 255}
}

func hsvToRGB(h, s, v float64) (uint8, uint8, uint8) {
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h/60.0, 2)-1))
	m := v - c
	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return uint8((r + m) * 255), uint8((g + m) * 255), uint8((b + m) * 255)
}

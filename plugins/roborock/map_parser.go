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
	"sort"
	"strings"
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

type segmentSummary struct {
	id         int
	pixelCount int
	minX       int
	minY       int
	maxX       int
	maxY       int
	sumX       int
	sumY       int
	label      string
}

func (s segmentSummary) centroidX() int {
	if s.pixelCount == 0 {
		return 0
	}
	return int(math.Round(float64(s.sumX) / float64(s.pixelCount)))
}

func (s segmentSummary) centroidY() int {
	if s.pixelCount == 0 {
		return 0
	}
	return int(math.Round(float64(s.sumY) / float64(s.pixelCount)))
}

func parseMapData(raw []byte, label string, labelMode string, labels map[uint32]string) (mapImage, []segmentSummary, error) {
	data := raw
	if len(raw) >= 2 && raw[0] == 0x1f && raw[1] == 0x8b {
		decompressed, err := gzipDecompress(raw)
		if err != nil {
			return mapImage{}, nil, err
		}
		data = decompressed
	}

	imageBlock, robotPos, carpetMap, err := extractMapImageBlock(data)
	if err != nil {
		return mapImage{}, nil, err
	}
	if imageBlock == nil {
		return mapImage{}, nil, fmt.Errorf("map image block not found")
	}

	segments := extractSegments(*imageBlock)
	applySegmentLabels(segments, labels)
	pngBytes, width, height, err := renderMapPNG(*imageBlock, robotPos, carpetMap, label, segments, labelMode)
	if err != nil {
		return mapImage{}, nil, err
	}
	return mapImage{png: pngBytes, width: width, height: height}, segments, nil
}

type mapImageBlock struct {
	left   int
	top    int
	width  int
	height int
	data   []byte
}

type mapPoint struct {
	x          int
	y          int
	headingDeg float64
	hasHeading bool
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
				pos := &mapPoint{x: x, y: y}
				if blockDataLen >= 12 {
					angleRaw := int32(int32le(data, 0x08))
					pos.headingDeg = decodeHeading(angleRaw)
					pos.hasHeading = true
				}
				robotPos = pos
			}
		}

		blockStart = blockStart + blockDataLen + int(byteToInt8(raw[blockStart+2]))
	}

	return imageBlock, robotPos, carpetMap, nil
}

func extractSegments(block mapImageBlock) []segmentSummary {
	width := block.width
	height := block.height
	if width == 0 || height == 0 || len(block.data) < width*height {
		return nil
	}
	segments := make(map[int]*segmentSummary)
	for imgY := 0; imgY < height; imgY++ {
		offset := width * imgY
		for imgX := 0; imgX < width; imgX++ {
			idx := imgX + offset
			pixelType := block.data[idx]
			if pixelType&0x07 != 0x07 {
				continue
			}
			segID := int(pixelType) >> 3
			x := imgX
			y := height - imgY - 1
			entry, ok := segments[segID]
			if !ok {
				entry = &segmentSummary{
					id:         segID,
					minX:       x,
					maxX:       x,
					minY:       y,
					maxY:       y,
					sumX:       x,
					sumY:       y,
					pixelCount: 1,
				}
				segments[segID] = entry
				continue
			}
			entry.pixelCount++
			entry.sumX += x
			entry.sumY += y
			if x < entry.minX {
				entry.minX = x
			}
			if x > entry.maxX {
				entry.maxX = x
			}
			if y < entry.minY {
				entry.minY = y
			}
			if y > entry.maxY {
				entry.maxY = y
			}
		}
	}
	result := make([]segmentSummary, 0, len(segments))
	for _, seg := range segments {
		result = append(result, *seg)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].id < result[j].id })
	return result
}

func renderMapPNG(block mapImageBlock, robot *mapPoint, carpetMap map[int]struct{}, label string, segments []segmentSummary, labelMode string) ([]byte, int, int, error) {
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
		drawRobotMarker(img, rx, height-ry-1, robot.headingDeg, robot.hasHeading)
		if strings.TrimSpace(label) != "" {
			drawRobotLabel(img, rx, height-ry-1, label)
		}
	}
	if labelMode != "" {
		drawSegmentLabels(img, segments, labelMode)
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

func byteToInt8(b byte) int8 {
	return int8(b)
}

func int16le(data []byte, offset int) uint16 {
	return uint16(data[offset]) | uint16(data[offset+1])<<8
}

func int32le(data []byte, offset int) uint32 {
	return uint32(data[offset]) | uint32(data[offset+1])<<8 | uint32(data[offset+2])<<16 | uint32(data[offset+3])<<24
}

func decodeHeading(angleRaw int32) float64 {
	angle := float64(angleRaw)
	if math.Abs(angle) > 360 {
		if math.Abs(angle) > 3600 {
			angle = angle / 100.0
		} else {
			angle = angle / 10.0
		}
	}
	for angle < 0 {
		angle += 360
	}
	for angle >= 360 {
		angle -= 360
	}
	return angle
}

func drawRobotMarker(img *image.RGBA, x, y int, headingDeg float64, hasHeading bool) {
	if x < 0 || y < 0 || x >= img.Bounds().Dx() || y >= img.Bounds().Dy() {
		return
	}
	outer := 8
	inner := 4
	for dy := -outer; dy <= outer; dy++ {
		for dx := -outer; dx <= outer; dx++ {
			xx := x + dx
			yy := y + dy
			if xx < 0 || yy < 0 || xx >= img.Bounds().Dx() || yy >= img.Bounds().Dy() {
				continue
			}
			dist := dx*dx + dy*dy
			if dist > outer*outer {
				continue
			}
			switch {
			case dist <= inner*inner:
				img.SetRGBA(xx, yy, colorRobotCenter())
			case dist <= (outer-2)*(outer-2):
				img.SetRGBA(xx, yy, colorRobotRing())
			default:
				img.SetRGBA(xx, yy, colorRobotOutline())
			}
		}
	}
	heading := 0.0
	if hasHeading {
		heading = headingDeg
	}
	drawRobotArrow(img, x, y, heading, outer)
}

func drawRobotArrow(img *image.RGBA, x, y int, headingDeg float64, radius int) {
	tip := pointF{0, float64(-radius - 6)}
	left := pointF{-6, float64(-radius + 2)}
	right := pointF{6, float64(-radius + 2)}

	angle := headingDeg * math.Pi / 180.0
	tip = rotatePoint(tip, angle)
	left = rotatePoint(left, angle)
	right = rotatePoint(right, angle)

	minX, maxX := minMaxFloat(tip.x, left.x, right.x)
	minY, maxY := minMaxFloat(tip.y, left.y, right.y)

	for yy := int(math.Floor(minY)); yy <= int(math.Ceil(maxY)); yy++ {
		for xx := int(math.Floor(minX)); xx <= int(math.Ceil(maxX)); xx++ {
			if !pointInTriangle(pointF{float64(xx), float64(yy)}, tip, left, right) {
				continue
			}
			px := x + xx
			py := y + yy
			if px < 0 || py < 0 || px >= img.Bounds().Dx() || py >= img.Bounds().Dy() {
				continue
			}
			img.SetRGBA(px, py, colorRobotArrow())
		}
	}
}

type pointF struct {
	x float64
	y float64
}

func rotatePoint(p pointF, angle float64) pointF {
	sin, cos := math.Sincos(angle)
	return pointF{
		x: p.x*cos - p.y*sin,
		y: p.x*sin + p.y*cos,
	}
}

func minMaxFloat(a, b, c float64) (float64, float64) {
	min := a
	max := a
	if b < min {
		min = b
	}
	if c < min {
		min = c
	}
	if b > max {
		max = b
	}
	if c > max {
		max = c
	}
	return min, max
}

func pointInTriangle(p, a, b, c pointF) bool {
	den := (b.y-c.y)*(a.x-c.x) + (c.x-b.x)*(a.y-c.y)
	if den == 0 {
		return false
	}
	w1 := ((b.y-c.y)*(p.x-c.x) + (c.x-b.x)*(p.y-c.y)) / den
	w2 := ((c.y-a.y)*(p.x-c.x) + (a.x-c.x)*(p.y-c.y)) / den
	w3 := 1 - w1 - w2
	return w1 >= 0 && w2 >= 0 && w3 >= 0
}

func drawRobotLabel(img *image.RGBA, x, y int, label string) {
	text := strings.ToUpper(label)
	const (
		charW   = 5
		charH   = 7
		spacing = 1
		padding = 2
	)
	if text == "" {
		return
	}
	textWidth := len([]rune(text))*(charW+spacing) - spacing
	if textWidth < 0 {
		textWidth = 0
	}
	boxW := textWidth + padding*2
	boxH := charH + padding*2
	labelX := x + 10
	labelY := y - boxH - 10
	if labelX+boxW >= img.Bounds().Dx() {
		labelX = img.Bounds().Dx() - boxW - 1
	}
	if labelX < 0 {
		labelX = 0
	}
	if labelY < 0 {
		labelY = 0
	}
	if labelY+boxH >= img.Bounds().Dy() {
		labelY = img.Bounds().Dy() - boxH - 1
	}
	drawFilledRect(img, labelX, labelY, boxW, boxH, colorLabelBg())
	drawText(img, labelX+padding, labelY+padding, text, colorLabelText())
}

func drawSegmentLabels(img *image.RGBA, segments []segmentSummary, labelMode string) {
	for _, seg := range segments {
		label := fmt.Sprintf("%d", seg.id)
		if labelMode == "names" && strings.TrimSpace(seg.label) != "" {
			label = strings.ReplaceAll(seg.label, "_", " ")
		}
		drawSegmentLabel(img, seg.centroidX(), seg.centroidY(), label)
	}
}

func drawSegmentLabel(img *image.RGBA, x, y int, label string) {
	text := strings.ToUpper(label)
	const (
		charW   = 5
		charH   = 7
		spacing = 1
		padding = 2
	)
	if text == "" {
		return
	}
	textWidth := len([]rune(text))*(charW+spacing) - spacing
	if textWidth < 0 {
		textWidth = 0
	}
	boxW := textWidth + padding*2
	boxH := charH + padding*2
	labelX := x - (boxW / 2)
	labelY := y - (boxH / 2)
	if labelX+boxW >= img.Bounds().Dx() {
		labelX = img.Bounds().Dx() - boxW - 1
	}
	if labelX < 0 {
		labelX = 0
	}
	if labelY < 0 {
		labelY = 0
	}
	if labelY+boxH >= img.Bounds().Dy() {
		labelY = img.Bounds().Dy() - boxH - 1
	}
	drawFilledRect(img, labelX, labelY, boxW, boxH, colorSegmentLabelBg())
	drawText(img, labelX+padding, labelY+padding, text, colorSegmentLabelText())
}

func drawFilledRect(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	if w <= 0 || h <= 0 {
		return
	}
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			if xx < 0 || yy < 0 || xx >= img.Bounds().Dx() || yy >= img.Bounds().Dy() {
				continue
			}
			img.SetRGBA(xx, yy, c)
		}
	}
}

func drawText(img *image.RGBA, x, y int, text string, c color.RGBA) {
	const (
		charW   = 5
		charH   = 7
		spacing = 1
	)
	offsetX := x
	for _, r := range text {
		glyph, ok := tinyFont[r]
		if !ok {
			glyph = tinyFont['?']
		}
		for row := 0; row < charH; row++ {
			rowBits := glyph[row]
			for col := 0; col < charW; col++ {
				mask := uint8(1 << (charW - 1 - col))
				if rowBits&mask == 0 {
					continue
				}
				px := offsetX + col
				py := y + row
				if px < 0 || py < 0 || px >= img.Bounds().Dx() || py >= img.Bounds().Dy() {
					continue
				}
				img.SetRGBA(px, py, c)
			}
		}
		offsetX += charW + spacing
	}
}

var tinyFont = map[rune][7]uint8{
	' ': {0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
	'-': {0x00, 0x00, 0x00, 0x1F, 0x00, 0x00, 0x00},
	'?': {0x0E, 0x11, 0x01, 0x02, 0x04, 0x00, 0x04},
	'0': {0x0E, 0x11, 0x13, 0x15, 0x19, 0x11, 0x0E},
	'1': {0x04, 0x0C, 0x04, 0x04, 0x04, 0x04, 0x0E},
	'2': {0x0E, 0x11, 0x01, 0x02, 0x04, 0x08, 0x1F},
	'3': {0x1F, 0x02, 0x04, 0x02, 0x01, 0x11, 0x0E},
	'4': {0x02, 0x06, 0x0A, 0x12, 0x1F, 0x02, 0x02},
	'5': {0x1F, 0x10, 0x1E, 0x01, 0x01, 0x11, 0x0E},
	'6': {0x06, 0x08, 0x10, 0x1E, 0x11, 0x11, 0x0E},
	'7': {0x1F, 0x01, 0x02, 0x04, 0x08, 0x08, 0x08},
	'8': {0x0E, 0x11, 0x11, 0x0E, 0x11, 0x11, 0x0E},
	'9': {0x0E, 0x11, 0x11, 0x0F, 0x01, 0x02, 0x0C},
	'A': {0x0E, 0x11, 0x11, 0x1F, 0x11, 0x11, 0x11},
	'B': {0x1E, 0x11, 0x11, 0x1E, 0x11, 0x11, 0x1E},
	'C': {0x0E, 0x11, 0x10, 0x10, 0x10, 0x11, 0x0E},
	'D': {0x1E, 0x11, 0x11, 0x11, 0x11, 0x11, 0x1E},
	'E': {0x1F, 0x10, 0x10, 0x1E, 0x10, 0x10, 0x1F},
	'F': {0x1F, 0x10, 0x10, 0x1E, 0x10, 0x10, 0x10},
	'G': {0x0E, 0x11, 0x10, 0x17, 0x11, 0x11, 0x0F},
	'H': {0x11, 0x11, 0x11, 0x1F, 0x11, 0x11, 0x11},
	'I': {0x0E, 0x04, 0x04, 0x04, 0x04, 0x04, 0x0E},
	'J': {0x07, 0x02, 0x02, 0x02, 0x02, 0x12, 0x0C},
	'K': {0x11, 0x12, 0x14, 0x18, 0x14, 0x12, 0x11},
	'L': {0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x1F},
	'M': {0x11, 0x1B, 0x15, 0x15, 0x11, 0x11, 0x11},
	'N': {0x11, 0x19, 0x15, 0x13, 0x11, 0x11, 0x11},
	'O': {0x0E, 0x11, 0x11, 0x11, 0x11, 0x11, 0x0E},
	'P': {0x1E, 0x11, 0x11, 0x1E, 0x10, 0x10, 0x10},
	'Q': {0x0E, 0x11, 0x11, 0x11, 0x15, 0x12, 0x0D},
	'R': {0x1E, 0x11, 0x11, 0x1E, 0x14, 0x12, 0x11},
	'S': {0x0F, 0x10, 0x10, 0x0E, 0x01, 0x01, 0x1E},
	'T': {0x1F, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04},
	'U': {0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x0E},
	'V': {0x11, 0x11, 0x11, 0x11, 0x11, 0x0A, 0x04},
	'W': {0x11, 0x11, 0x11, 0x15, 0x15, 0x15, 0x0A},
	'X': {0x11, 0x11, 0x0A, 0x04, 0x0A, 0x11, 0x11},
	'Y': {0x11, 0x11, 0x0A, 0x04, 0x04, 0x04, 0x04},
	'Z': {0x1F, 0x01, 0x02, 0x04, 0x08, 0x10, 0x1F},
}

func colorOutside() color.RGBA { return color.RGBA{0, 0, 0, 0} }
func colorWall() color.RGBA    { return color.RGBA{40, 40, 40, 255} }
func colorWallV2() color.RGBA  { return color.RGBA{60, 60, 60, 255} }
func colorInside() color.RGBA  { return color.RGBA{230, 230, 230, 255} }
func colorScan() color.RGBA    { return color.RGBA{200, 220, 255, 255} }
func colorGreyWall() color.RGBA {
	return color.RGBA{90, 90, 90, 255}
}
func colorUnknown() color.RGBA      { return color.RGBA{180, 80, 180, 255} }
func colorCarpet() color.RGBA       { return color.RGBA{220, 160, 90, 255} }
func colorRobotCenter() color.RGBA  { return color.RGBA{235, 45, 45, 255} }
func colorRobotRing() color.RGBA    { return color.RGBA{255, 255, 255, 255} }
func colorRobotOutline() color.RGBA { return color.RGBA{20, 20, 20, 255} }
func colorRobotArrow() color.RGBA   { return color.RGBA{20, 20, 20, 255} }
func colorLabelBg() color.RGBA      { return color.RGBA{10, 10, 10, 200} }
func colorLabelText() color.RGBA    { return color.RGBA{255, 255, 255, 255} }
func colorSegmentLabelBg() color.RGBA {
	return color.RGBA{10, 10, 10, 220}
}
func colorSegmentLabelText() color.RGBA { return color.RGBA{255, 255, 255, 255} }

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

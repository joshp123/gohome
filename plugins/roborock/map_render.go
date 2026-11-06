package roborock

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"sort"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomedium"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

// The map answers three questions at a glance: which rooms the vacuum knows
// and what they are called, where the vacuum and its dock are, and where the
// last run went. Everything else in the device's map is left out.
const (
	outputPixelsPerCell   = 2  // one cell is 50 mm
	cropMarginCells       = 8  // 40 cm of empty space around the floor plan
	roomLabelFontSize     = 22 // output pixels
	roomLabelPaddingX     = 9
	roomLabelPaddingY     = 5
	roomLabelCornerRadius = 7
	roomLabelGap          = 6
	vacuumMarkerRadius    = 10
	dockMarkerHalfSide    = 13
	markerOutlineWidth    = 2.5
	pathStrokeRadius      = 1.1
)

var (
	roomPalette = []color.NRGBA{
		{0x8a, 0xb4, 0xf8, 0xff}, // blue
		{0xf6, 0xc2, 0x6b, 0xff}, // amber
		{0x81, 0xc9, 0x95, 0xff}, // green
		{0xc5, 0x8a, 0xf9, 0xff}, // purple
		{0xf2, 0x8b, 0x82, 0xff}, // salmon
		{0x78, 0xd9, 0xec, 0xff}, // cyan
		{0xfd, 0xd6, 0x63, 0xff}, // yellow
		{0xff, 0x8b, 0xcb, 0xff}, // pink
	}
	colourWall             = color.NRGBA{0xe4, 0xe7, 0xec, 0xff}
	colourUnassignedFloor  = color.NRGBA{0xb4, 0xba, 0xc4, 0xff}
	colourObstacle         = color.NRGBA{0x4a, 0x50, 0x5c, 0xff}
	colourLastRunPath      = color.NRGBA{0xff, 0xff, 0xff, 0xb0}
	colourMarkerOutline    = color.NRGBA{0x18, 0x1b, 0x1f, 0xff}
	colourVacuum           = color.NRGBA{0xff, 0xff, 0xff, 0xff}
	colourDock             = color.NRGBA{0x5a, 0xa4, 0x69, 0xff}
	colourRoomLabel        = color.NRGBA{0x18, 0x1b, 0x1f, 0xe6}
	colourRoomLabelText    = color.NRGBA{0xf4, 0xf5, 0xf7, 0xff}
	roomLabelFace          font.Face
	roomLabelFaceLoadError error
	roomLabelFaceOnce      sync.Once
)

func loadRoomLabelFace() (font.Face, error) {
	roomLabelFaceOnce.Do(func() {
		parsed, err := opentype.Parse(gomedium.TTF)
		if err != nil {
			roomLabelFaceLoadError = err
			return
		}
		roomLabelFace, roomLabelFaceLoadError = opentype.NewFace(parsed, &opentype.FaceOptions{
			Size: roomLabelFontSize, DPI: 72, Hinting: font.HintingFull,
		})
	})
	return roomLabelFace, roomLabelFaceLoadError
}

// renderVacuumMap draws the map as a PNG. labelMode is "names" (room names),
// "segments" (room ids, for configuring names) or "" (no labels).
func renderVacuumMap(vacuumMapData vacuumMap, labelMode string, showLastRunPath bool) ([]byte, error) {
	grid := vacuumMapData.floor
	crop, ok := floorPlanBounds(vacuumMapData)
	if !ok {
		return nil, fmt.Errorf("map has no floor plan yet")
	}
	canvas := image.NewNRGBA(image.Rect(0, 0, crop.Dx()*outputPixelsPerCell, crop.Dy()*outputPixelsPerCell))
	toOutput := func(position mapPosition) (float64, float64) {
		return (position.x - float64(crop.Min.X) + 0.5) * outputPixelsPerCell,
			(position.y - float64(crop.Min.Y) + 0.5) * outputPixelsPerCell
	}

	roomColours := make(map[int]color.NRGBA, len(vacuumMapData.rooms))
	for index, room := range vacuumMapData.rooms {
		roomColours[room.id] = roomPalette[index%len(roomPalette)]
	}
	for y := max(crop.Min.Y, 0); y < min(crop.Max.Y, grid.height); y++ {
		for x := max(crop.Min.X, 0); x < min(crop.Max.X, grid.width); x++ {
			cellColour, visible := floorCellColour(grid, vacuumMapData.carpetCells, roomColours, x, y)
			if !visible {
				continue
			}
			outputX := (x - crop.Min.X) * outputPixelsPerCell
			outputY := (y - crop.Min.Y) * outputPixelsPerCell
			draw.Draw(canvas, image.Rect(outputX, outputY, outputX+outputPixelsPerCell, outputY+outputPixelsPerCell),
				image.NewUniform(cellColour), image.Point{}, draw.Src)
		}
	}

	if showLastRunPath && len(vacuumMapData.lastRunPath) > 1 {
		drawLastRunPath(canvas, vacuumMapData.lastRunPath, toOutput)
	}

	var occupied []image.Rectangle
	if vacuumMapData.dock != nil {
		x, y := toOutput(*vacuumMapData.dock)
		half := float64(dockMarkerHalfSide)
		fillPolygon(canvas, colourMarkerOutline, roundedRectangle(x-half-markerOutlineWidth, y-half-markerOutlineWidth, x+half+markerOutlineWidth, y+half+markerOutlineWidth, 4))
		fillPolygon(canvas, colourDock, roundedRectangle(x-half, y-half, x+half, y+half, 3))
		occupied = append(occupied, markerBounds(x, y, half+markerOutlineWidth))
	}
	if vacuumMapData.vacuum != nil {
		x, y := toOutput(*vacuumMapData.vacuum)
		fillPolygon(canvas, colourMarkerOutline, circle(x, y, vacuumMarkerRadius+markerOutlineWidth))
		fillPolygon(canvas, colourVacuum, circle(x, y, vacuumMarkerRadius))
		fillPolygon(canvas, colourMarkerOutline, circle(x, y, 3))
		occupied = append(occupied, markerBounds(x, y, vacuumMarkerRadius+markerOutlineWidth))
	}

	if labelMode != "" {
		if err := drawRoomLabels(canvas, vacuumMapData, crop, labelMode, occupied); err != nil {
			return nil, err
		}
	}

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, canvas); err != nil {
		return nil, err
	}
	return encoded.Bytes(), nil
}

// floorPlanBounds is the smallest rectangle of cells holding the floor plan,
// the dock and the vacuum, plus a margin.
func floorPlanBounds(vacuumMapData vacuumMap) (image.Rectangle, bool) {
	grid := vacuumMapData.floor
	bounds := image.Rectangle{}
	found := false
	include := func(x, y int) {
		cell := image.Rect(x, y, x+1, y+1)
		if !found {
			bounds, found = cell, true
			return
		}
		bounds = bounds.Union(cell)
	}
	for y := 0; y < grid.height; y++ {
		for x := 0; x < grid.width; x++ {
			if grid.pixelAt(x, y) != floorPixelOutside {
				include(x, y)
			}
		}
	}
	if !found {
		return image.Rectangle{}, false
	}
	for _, position := range []*mapPosition{vacuumMapData.dock, vacuumMapData.vacuum} {
		if position != nil {
			include(int(math.Floor(position.x)), int(math.Floor(position.y)))
		}
	}
	return bounds.Inset(-cropMarginCells), true
}

func floorCellColour(grid floorGrid, carpetCells map[int]bool, roomColours map[int]color.NRGBA, x, y int) (color.NRGBA, bool) {
	pixel := grid.pixelAt(x, y)
	if roomID, ok := roomIDForPixel(pixel); ok {
		roomColour := roomColours[roomID]
		// Carpet shows as faint diagonal stripes in the room's colour.
		if carpetCells[grid.cellIndex(x, y)] && (x+y)%4 < 2 {
			return shade(roomColour, 0.84), true
		}
		return roomColour, true
	}
	if roomID, ok := roomIDForWallOrObstacle(pixel); ok && pixel&0x07 == 0x01 {
		// Walls the vacuum attributes to a room, including furniture it
		// bumped into, show as a darker tone of that room.
		return shade(roomColours[roomID], 0.68), true
	}
	switch {
	case pixel == floorPixelOutside:
		return color.NRGBA{}, false
	case pixel == floorPixelWall || pixel&0x07 == 0x01:
		return colourWall, true
	case pixel == floorPixelScanned || pixel == floorPixelUnassignedFloor:
		return colourUnassignedFloor, true
	default:
		// Furniture and other obstacles the vacuum detected.
		return colourObstacle, true
	}
}

func shade(base color.NRGBA, factor float64) color.NRGBA {
	return color.NRGBA{
		R: uint8(float64(base.R) * factor),
		G: uint8(float64(base.G) * factor),
		B: uint8(float64(base.B) * factor),
		A: base.A,
	}
}

func drawLastRunPath(canvas *image.NRGBA, path []mapPosition, toOutput func(mapPosition) (float64, float64)) {
	// Stroke into a mask first so overlapping passes do not build up opacity.
	mask := image.NewAlpha(canvas.Bounds())
	for index := 1; index < len(path); index++ {
		startX, startY := toOutput(path[index-1])
		endX, endY := toOutput(path[index])
		steps := int(math.Ceil(math.Hypot(endX-startX, endY-startY) * 2))
		for step := 0; step <= steps; step++ {
			fraction := 0.0
			if steps > 0 {
				fraction = float64(step) / float64(steps)
			}
			stampDisc(mask, startX+(endX-startX)*fraction, startY+(endY-startY)*fraction, pathStrokeRadius)
		}
	}
	draw.DrawMask(canvas, canvas.Bounds(), image.NewUniform(colourLastRunPath), image.Point{}, mask, image.Point{}, draw.Over)
}

func stampDisc(mask *image.Alpha, centreX, centreY, radius float64) {
	for y := int(math.Floor(centreY - radius)); y <= int(math.Ceil(centreY+radius)); y++ {
		for x := int(math.Floor(centreX - radius)); x <= int(math.Ceil(centreX+radius)); x++ {
			if math.Hypot(float64(x)+0.5-centreX, float64(y)+0.5-centreY) <= radius {
				mask.SetAlpha(x, y, color.Alpha{A: 0xff})
			}
		}
	}
}

func markerBounds(x, y, radius float64) image.Rectangle {
	return image.Rect(int(x-radius), int(y-radius), int(math.Ceil(x+radius)), int(math.Ceil(y+radius)))
}

func circle(centreX, centreY, radius float64) []mapPosition {
	const corners = 48
	points := make([]mapPosition, 0, corners)
	for index := 0; index < corners; index++ {
		angle := 2 * math.Pi * float64(index) / corners
		points = append(points, mapPosition{x: centreX + radius*math.Cos(angle), y: centreY + radius*math.Sin(angle)})
	}
	return points
}

func roundedRectangle(left, top, right, bottom, radius float64) []mapPosition {
	corners := []struct{ x, y, startAngle float64 }{
		{right - radius, top + radius, -math.Pi / 2},
		{right - radius, bottom - radius, 0},
		{left + radius, bottom - radius, math.Pi / 2},
		{left + radius, top + radius, math.Pi},
	}
	var points []mapPosition
	for _, corner := range corners {
		for step := 0; step <= 6; step++ {
			angle := corner.startAngle + (math.Pi/2)*float64(step)/6
			points = append(points, mapPosition{x: corner.x + radius*math.Cos(angle), y: corner.y + radius*math.Sin(angle)})
		}
	}
	return points
}

func fillPolygon(canvas *image.NRGBA, fill color.NRGBA, points []mapPosition) {
	bounds := canvas.Bounds()
	rasterizer := vector.NewRasterizer(bounds.Dx(), bounds.Dy())
	rasterizer.MoveTo(float32(points[0].x), float32(points[0].y))
	for _, point := range points[1:] {
		rasterizer.LineTo(float32(point.x), float32(point.y))
	}
	rasterizer.ClosePath()
	rasterizer.Draw(canvas, bounds, image.NewUniform(fill), image.Point{})
}

func roomLabelText(room segmentSummary, labelMode string) string {
	if labelMode == "segments" || strings.TrimSpace(room.label) == "" {
		if labelMode == "segments" {
			return fmt.Sprintf("%d", room.id)
		}
		return fmt.Sprintf("Room %d", room.id)
	}
	text := strings.TrimSpace(strings.ReplaceAll(room.label, "_", " "))
	return strings.ToUpper(text[:1]) + text[1:]
}

// drawRoomLabels puts each room's label at the point deepest inside the room
// whose label box overlaps no marker or earlier label. Rooms with the least
// space are placed first so that large rooms give way.
func drawRoomLabels(canvas *image.NRGBA, vacuumMapData vacuumMap, crop image.Rectangle, labelMode string, occupied []image.Rectangle) error {
	face, err := loadRoomLabelFace()
	if err != nil {
		return err
	}
	grid := vacuumMapData.floor
	depth := roomDepths(grid)
	type labelCandidate struct {
		x, y  int
		depth int
	}
	type roomLabel struct {
		text       string
		candidates []labelCandidate
	}
	labels := make([]roomLabel, 0, len(vacuumMapData.rooms))
	candidatesByRoom := make(map[int][]labelCandidate)
	for y := max(crop.Min.Y, 0); y < min(crop.Max.Y, grid.height); y++ {
		for x := max(crop.Min.X, 0); x < min(crop.Max.X, grid.width); x++ {
			if roomID, ok := roomIDForPixel(grid.pixelAt(x, y)); ok {
				candidatesByRoom[roomID] = append(candidatesByRoom[roomID], labelCandidate{x: x, y: y, depth: depth[y*grid.width+x]})
			}
		}
	}
	for _, room := range vacuumMapData.rooms {
		candidates := candidatesByRoom[room.id]
		if len(candidates) == 0 {
			continue
		}
		centroidX, centroidY := room.centroidX(), room.centroidY()
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].depth != candidates[j].depth {
				return candidates[i].depth > candidates[j].depth
			}
			distanceI := (candidates[i].x-centroidX)*(candidates[i].x-centroidX) + (candidates[i].y-centroidY)*(candidates[i].y-centroidY)
			distanceJ := (candidates[j].x-centroidX)*(candidates[j].x-centroidX) + (candidates[j].y-centroidY)*(candidates[j].y-centroidY)
			return distanceI < distanceJ
		})
		labels = append(labels, roomLabel{text: roomLabelText(room, labelMode), candidates: candidates})
	}
	sort.SliceStable(labels, func(i, j int) bool { return labels[i].candidates[0].depth < labels[j].candidates[0].depth })

	metrics := face.Metrics()
	textHeight := (metrics.Ascent + metrics.Descent).Ceil()
	for _, label := range labels {
		boxWidth := font.MeasureString(face, label.text).Ceil() + 2*roomLabelPaddingX
		boxHeight := textHeight + 2*roomLabelPaddingY
		boxAt := func(candidate labelCandidate) image.Rectangle {
			centreX := (candidate.x-crop.Min.X)*outputPixelsPerCell + outputPixelsPerCell/2
			centreY := (candidate.y-crop.Min.Y)*outputPixelsPerCell + outputPixelsPerCell/2
			return image.Rect(centreX-boxWidth/2, centreY-boxHeight/2, centreX-boxWidth/2+boxWidth, centreY-boxHeight/2+boxHeight)
		}
		chosen := boxAt(label.candidates[0])
		for _, candidate := range label.candidates {
			box := boxAt(candidate)
			if box.In(canvas.Bounds()) && !overlapsAny(box.Inset(-roomLabelGap), occupied) {
				chosen = box
				break
			}
		}
		occupied = append(occupied, chosen)
		fillPolygon(canvas, colourRoomLabel, roundedRectangle(float64(chosen.Min.X), float64(chosen.Min.Y), float64(chosen.Max.X), float64(chosen.Max.Y), roomLabelCornerRadius))
		drawer := font.Drawer{Dst: canvas, Src: image.NewUniform(colourRoomLabelText), Face: face,
			Dot: fixed.P(chosen.Min.X+roomLabelPaddingX, chosen.Min.Y+roomLabelPaddingY+metrics.Ascent.Ceil())}
		drawer.DrawString(label.text)
	}
	return nil
}

func overlapsAny(box image.Rectangle, others []image.Rectangle) bool {
	for _, other := range others {
		if box.Overlaps(other) {
			return true
		}
	}
	return false
}

// roomDepths gives each room cell its distance, in cells, to the nearest cell
// outside its room (a two-pass chessboard distance transform).
func roomDepths(grid floorGrid) []int {
	depth := make([]int, grid.width*grid.height)
	roomAt := func(x, y int) int {
		if x < 0 || y < 0 || x >= grid.width || y >= grid.height {
			return -1
		}
		pixel := grid.pixelAt(x, y)
		if roomID, ok := roomIDForPixel(pixel); ok {
			return roomID
		}
		// Furniture inside a room should not push its label aside.
		if roomID, ok := roomIDForWallOrObstacle(pixel); ok {
			return roomID
		}
		return -1
	}
	neighbourDepth := func(roomID, x, y int) int {
		if roomAt(x, y) != roomID {
			return 0
		}
		return depth[y*grid.width+x]
	}
	for y := 0; y < grid.height; y++ {
		for x := 0; x < grid.width; x++ {
			roomID := roomAt(x, y)
			if roomID < 0 {
				continue
			}
			depth[y*grid.width+x] = 1 + min(neighbourDepth(roomID, x-1, y), neighbourDepth(roomID, x-1, y-1),
				neighbourDepth(roomID, x, y-1), neighbourDepth(roomID, x+1, y-1))
		}
	}
	for y := grid.height - 1; y >= 0; y-- {
		for x := grid.width - 1; x >= 0; x-- {
			roomID := roomAt(x, y)
			if roomID < 0 {
				continue
			}
			index := y*grid.width + x
			depth[index] = min(depth[index], 1+min(neighbourDepth(roomID, x+1, y), neighbourDepth(roomID, x+1, y+1),
				neighbourDepth(roomID, x, y+1), neighbourDepth(roomID, x-1, y+1)))
		}
	}
	return depth
}

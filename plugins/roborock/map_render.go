package roborock

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
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

// The map answers four questions at a glance: which rooms the vacuum knows
// and what they are called, where the vacuum and its dock are, where the
// last run went, and where the vacuum is told not to go.
const (
	outputPixelsPerCell   = 4 // one cell is 50 mm, so one pixel is 1.25 cm
	cropMarginCells       = 8 // 40 cm of empty space around the floor plan
	roomLabelFontSize     = 36
	roomLabelPaddingX     = 14
	roomLabelPaddingY     = 8
	roomLabelCornerRadius = 12
	roomLabelGap          = 10
	vacuumMarkerRadius    = 17
	dockMarkerHalfSide    = 15
	markerOutlineWidth    = 4
	pathStrokeWidth       = 3
	roomSeamWidth         = 2.5 // background left between two rooms that touch
	zoneOutlineWidth      = 3
	virtualWallWidth      = 5
	speckleMaxCells       = 3 // 15 cm: chair legs and lidar noise, not furniture
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
	colourWall             = color.NRGBA{0x8a, 0x91, 0x9c, 0xff}
	colourObstacle         = color.NRGBA{0x2f, 0x34, 0x3c, 0xff}
	colourUnassignedFloor  = color.NRGBA{0xb4, 0xba, 0xc4, 0xff}
	colourLastRunPath      = color.NRGBA{0xff, 0xff, 0xff, 0xc0}
	colourNoGoFill         = color.NRGBA{0xe0, 0x40, 0x40, 0x50}
	colourNoGoOutline      = color.NRGBA{0xe0, 0x40, 0x40, 0xff}
	colourNoMopFill        = color.NRGBA{0xb8, 0x77, 0xd9, 0x50}
	colourNoMopOutline     = color.NRGBA{0xb8, 0x77, 0xd9, 0xff}
	colourVirtualWall      = color.NRGBA{0xe0, 0x40, 0x40, 0xff}
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
	crop, ok := floorPlanBounds(vacuumMapData)
	if !ok {
		return nil, fmt.Errorf("map has no floor plan yet")
	}
	canvas := image.NewNRGBA(image.Rect(0, 0, crop.Dx()*outputPixelsPerCell, crop.Dy()*outputPixelsPerCell))
	toOutput := func(position mapPosition) mapPosition {
		return mapPosition{
			x: (position.x - float64(crop.Min.X) + 0.5) * outputPixelsPerCell,
			y: (position.y - float64(crop.Min.Y) + 0.5) * outputPixelsPerCell,
		}
	}
	allToOutput := func(positions []mapPosition) []mapPosition {
		converted := make([]mapPosition, len(positions))
		for index, position := range positions {
			converted[index] = toOutput(position)
		}
		return converted
	}

	roomColours := make(map[int]color.NRGBA, len(vacuumMapData.rooms))
	for index, room := range vacuumMapData.rooms {
		roomColours[room.id] = roomPalette[index%len(roomPalette)]
	}
	paintFloor(canvas, vacuumMapData, crop, roomColours)

	for _, zones := range []struct {
		areas         []mapArea
		fill, outline color.NRGBA
	}{
		{vacuumMapData.noGoAreas, colourNoGoFill, colourNoGoOutline},
		{vacuumMapData.noMopAreas, colourNoMopFill, colourNoMopOutline},
	} {
		for _, area := range zones.areas {
			corners := allToOutput(area[:])
			fillPolygon(canvas, zones.fill, corners)
			strokePolyline(canvas, zones.outline, zoneOutlineWidth, append(corners, corners[0]))
		}
	}
	for _, wall := range vacuumMapData.virtualWalls {
		strokePolyline(canvas, colourVirtualWall, virtualWallWidth, allToOutput(wall[:]))
	}

	if showLastRunPath && len(vacuumMapData.lastRunPath) > 1 {
		strokePolyline(canvas, colourLastRunPath, pathStrokeWidth, allToOutput(vacuumMapData.lastRunPath))
	}

	var occupied []image.Rectangle
	if vacuumMapData.dock != nil {
		dock := toOutput(*vacuumMapData.dock)
		half := float64(dockMarkerHalfSide)
		fillPolygon(canvas, colourMarkerOutline, roundedRectangle(dock.x-half-markerOutlineWidth, dock.y-half-markerOutlineWidth, dock.x+half+markerOutlineWidth, dock.y+half+markerOutlineWidth, 6))
		fillPolygon(canvas, colourDock, roundedRectangle(dock.x-half, dock.y-half, dock.x+half, dock.y+half, 4))
		occupied = append(occupied, markerBounds(dock.x, dock.y, half+markerOutlineWidth))
	}
	if vacuumMapData.vacuum != nil {
		vacuum := toOutput(*vacuumMapData.vacuum)
		drawVacuum(canvas, vacuum, vacuumMapData.vacuumHeadingDegrees)
		occupied = append(occupied, markerBounds(vacuum.x, vacuum.y, vacuumMarkerRadius+markerOutlineWidth))
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

// cellClass is what a floor cell is drawn as. Rooms are cellRoom plus the room id.
type cellClass uint8

const (
	cellOutside cellClass = iota
	cellWall
	cellObstacle
	cellUnassignedFloor
	cellRoom
)

func classifyCell(pixel byte) cellClass {
	if roomID, ok := roomIDForPixel(pixel); ok {
		return cellRoom + cellClass(roomID)
	}
	switch {
	case pixel == floorPixelOutside:
		return cellOutside
	case pixel == floorPixelScanned || pixel == floorPixelUnassignedFloor:
		return cellUnassignedFloor
	case pixel&0x07 == 0x01:
		// Walls, including the ones the vacuum attributes to a room.
		return cellWall
	default:
		// Furniture and other obstacles, and what the lidar saw beyond them.
		return cellObstacle
	}
}

// floorClasses classifies every floor cell in image order (y*width+x) and
// paints over furniture specks: a blob of at most speckleMaxCells wall or
// obstacle cells with one room all round it becomes that room.
func floorClasses(grid floorGrid) []cellClass {
	classes := make([]cellClass, grid.width*grid.height)
	for y := 0; y < grid.height; y++ {
		for x := 0; x < grid.width; x++ {
			classes[y*grid.width+x] = classifyCell(grid.pixelAt(x, y))
		}
	}
	isBlocked := func(class cellClass) bool { return class == cellWall || class == cellObstacle }
	visited := make([]bool, len(classes))
	for start, class := range classes {
		if visited[start] || !isBlocked(class) {
			continue
		}
		blob := []int{start}
		visited[start] = true
		for next := 0; next < len(blob); next++ {
			x, y := blob[next]%grid.width, blob[next]/grid.width
			for _, step := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				neighbourX, neighbourY := x+step[0], y+step[1]
				if neighbourX < 0 || neighbourY < 0 || neighbourX >= grid.width || neighbourY >= grid.height {
					continue
				}
				neighbour := neighbourY*grid.width + neighbourX
				if !visited[neighbour] && isBlocked(classes[neighbour]) {
					visited[neighbour] = true
					blob = append(blob, neighbour)
				}
			}
		}
		if len(blob) > speckleMaxCells {
			continue
		}
		if room, ok := surroundingRoom(classes, grid, blob, isBlocked); ok {
			for _, cell := range blob {
				classes[cell] = room
			}
		}
	}
	return classes
}

// surroundingRoom reports the room on every side of a blob, if there is only one.
func surroundingRoom(classes []cellClass, grid floorGrid, blob []int, isBlocked func(cellClass) bool) (cellClass, bool) {
	room := cellOutside
	for _, cell := range blob {
		x, y := cell%grid.width, cell/grid.width
		for neighbourY := y - 1; neighbourY <= y+1; neighbourY++ {
			for neighbourX := x - 1; neighbourX <= x+1; neighbourX++ {
				if neighbourX < 0 || neighbourY < 0 || neighbourX >= grid.width || neighbourY >= grid.height {
					return cellOutside, false
				}
				class := classes[neighbourY*grid.width+neighbourX]
				if isBlocked(class) {
					continue
				}
				if class < cellRoom || (room != cellOutside && class != room) {
					return cellOutside, false
				}
				room = class
			}
		}
	}
	return room, room != cellOutside
}

type classWeight struct {
	class        cellClass
	weight       float64
	carpetWeight float64
}

// classTally sums the weight of each class over the cells near one pixel.
type classTally struct {
	entries [16]classWeight
	count   int
	total   float64
}

func (t *classTally) add(class cellClass, weight float64, carpet bool) {
	entry := 0
	for entry < t.count && t.entries[entry].class != class {
		entry++
	}
	if entry == t.count {
		t.entries[entry] = classWeight{class: class}
		t.count++
	}
	t.entries[entry].weight += weight
	if carpet {
		t.entries[entry].carpetWeight += weight
	}
	t.total += weight
}

// topTwo returns the two classes with the most weight; a missing runner-up
// is outside with no weight.
func (t *classTally) topTwo() (winner, runnerUp classWeight) {
	for _, entry := range t.entries[:t.count] {
		if entry.weight > winner.weight {
			winner, runnerUp = entry, winner
		} else if entry.weight > runnerUp.weight {
			runnerUp = entry
		}
	}
	return winner, runnerUp
}

// cubicBSplineWeights weighs the four cells at offsets -1, 0, 1 and 2 from a
// point a fraction of the way from cell 0 to cell 1.
func cubicBSplineWeights(fraction float64) [4]float64 {
	inverse := 1 - fraction
	return [4]float64{
		inverse * inverse * inverse / 6,
		(3*fraction*fraction*fraction - 6*fraction*fraction + 4) / 6,
		(-3*fraction*fraction*fraction + 3*fraction*fraction + 3*fraction + 1) / 6,
		fraction * fraction * fraction / 6,
	}
}

// paintFloor draws the floor cells with smooth edges, in two layers.
//
// Floor (rooms, unassigned floor and outside) is weighed over the 4 x 4
// cells round each output pixel with a cubic B-spline, which turns the
// cells' staircases into smooth curves. Where two rooms meet, a thin seam
// of background is left between them, as in the Roborock app.
//
// Walls and obstacles are one cell thin, which that wider kernel would
// erode, so they are weighed bilinearly over the nearest 2 x 2 cells and
// drawn on top.
//
// In both layers the class with the most weight wins, and its edge is
// blended with the runner-up over about one output pixel.
func paintFloor(canvas *image.NRGBA, vacuumMapData vacuumMap, crop image.Rectangle, roomColours map[int]color.NRGBA) {
	grid := vacuumMapData.floor
	classes := floorClasses(grid)
	classAt := func(x, y int) cellClass {
		if x < 0 || y < 0 || x >= grid.width || y >= grid.height {
			return cellOutside
		}
		return classes[y*grid.width+x]
	}
	carpet := make([]bool, len(classes))
	for y := 0; y < grid.height; y++ {
		for x := 0; x < grid.width; x++ {
			carpet[y*grid.width+x] = vacuumMapData.carpetCells[grid.cellIndex(x, y)]
		}
	}
	isCarpet := func(x, y int) bool {
		return x >= 0 && y >= 0 && x < grid.width && y < grid.height && carpet[y*grid.width+x]
	}
	isWall := func(class cellClass) bool { return class == cellWall }
	// Most of the margin and the space between wings is outside; skip it.
	isEmptyAround := func(left, top int) bool {
		for y := top - 1; y <= top+2; y++ {
			for x := left - 1; x <= left+2; x++ {
				if classAt(x, y) != cellOutside {
					return false
				}
			}
		}
		return true
	}
	classColour := func(class cellClass) color.NRGBA {
		switch class {
		case cellOutside:
			return color.NRGBA{}
		case cellWall:
			return colourWall
		case cellObstacle:
			return colourObstacle
		case cellUnassignedFloor:
			return colourUnassignedFloor
		default:
			return roomColours[int(class-cellRoom)]
		}
	}

	const scale = float64(outputPixelsPerCell)
	// The smoothed step a B-spline makes of an edge rises by 2/3 per cell at
	// the edge, so the weight difference rises by 4/3 per cell.
	const bSplineCellsPerWeight = 0.75
	bounds := canvas.Bounds()
	cellPosition := func(output, cropStart int) (int, float64) {
		position := float64(cropStart) + (float64(output)+0.5)/scale - 0.5
		cell := int(math.Floor(position))
		return cell, position - float64(cell)
	}
	for outputY := 0; outputY < bounds.Dy(); outputY++ {
		top, fractionY := cellPosition(outputY, crop.Min.Y)
		bilinearY := [2]float64{1 - fractionY, fractionY}
		splineY := cubicBSplineWeights(fractionY)
		for outputX := 0; outputX < bounds.Dx(); outputX++ {
			left, fractionX := cellPosition(outputX, crop.Min.X)
			bilinearX := [2]float64{1 - fractionX, fractionX}
			splineX := cubicBSplineWeights(fractionX)

			if isEmptyAround(left, top) {
				continue
			}
			var nearest, floor classTally
			for row := 0; row < 4; row++ {
				for column := 0; column < 4; column++ {
					x, y := left+column-1, top+row-1
					class := classAt(x, y)
					if row == 1 || row == 2 {
						if column == 1 || column == 2 {
							nearest.add(class, bilinearX[column-1]*bilinearY[row-1], false)
						}
					}
					if !isWall(class) {
						floor.add(class, splineX[column]*splineY[row], isCarpet(x, y))
					}
				}
			}

			floorColour := color.NRGBA{}
			if floor.total > 0 {
				winner, runnerUp := floor.topTwo()
				distance := (winner.weight - runnerUp.weight) / floor.total * bSplineCellsPerWeight * scale
				winnerColour := classColour(winner.class)
				if winner.class >= cellRoom && winner.carpetWeight > winner.weight/2 && (outputX+outputY)%8 < 3 {
					// Carpet shows as fine diagonal stripes in the room's colour.
					winnerColour = shade(winnerColour, 0.88)
				}
				if winner.class >= cellRoom && runnerUp.class >= cellRoom && runnerUp.weight > 0 {
					floorColour = winnerColour
					floorColour.A = uint8(math.Round(float64(winnerColour.A) * clamp01(distance-roomSeamWidth/2+0.5)))
				} else {
					floorColour = mixColours(classColour(runnerUp.class), winnerColour, clamp01(distance+0.5))
				}
			}

			winner, runnerUp := nearest.topTwo()
			distance := (winner.weight - runnerUp.weight) * scale / 2
			pixel := floorColour
			switch {
			case isWall(winner.class) && floor.total == 0:
				pixel = classColour(winner.class)
			case isWall(winner.class):
				pixel = mixColours(floorColour, classColour(winner.class), clamp01(distance+0.5))
			case isWall(runnerUp.class):
				pixel = mixColours(floorColour, classColour(runnerUp.class), clamp01(0.5-distance))
			}
			if pixel.A != 0 {
				canvas.SetNRGBA(outputX, outputY, pixel)
			}
		}
	}
}

func clamp01(value float64) float64 {
	return math.Max(0, math.Min(1, value))
}

// mixColours blends from a to b by fraction, with correct alpha.
func mixColours(a, b color.NRGBA, fraction float64) color.NRGBA {
	alphaA, alphaB := float64(a.A)/255, float64(b.A)/255
	alpha := alphaA*(1-fraction) + alphaB*fraction
	if alpha == 0 {
		return color.NRGBA{}
	}
	channel := func(channelA, channelB uint8) uint8 {
		return uint8(math.Round((float64(channelA)*alphaA*(1-fraction) + float64(channelB)*alphaB*fraction) / alpha))
	}
	return color.NRGBA{channel(a.R, b.R), channel(a.G, b.G), channel(a.B, b.B), uint8(math.Round(alpha * 255))}
}

func shade(base color.NRGBA, factor float64) color.NRGBA {
	return color.NRGBA{
		R: uint8(float64(base.R) * factor),
		G: uint8(float64(base.G) * factor),
		B: uint8(float64(base.B) * factor),
		A: base.A,
	}
}

// drawVacuum draws the vacuum as a white disc. A dark arrowhead points where
// it faces; without a heading, a dot marks its centre.
func drawVacuum(canvas *image.NRGBA, centre mapPosition, headingDegrees *float64) {
	fillPolygon(canvas, colourMarkerOutline, circle(centre.x, centre.y, vacuumMarkerRadius+markerOutlineWidth))
	fillPolygon(canvas, colourVacuum, circle(centre.x, centre.y, vacuumMarkerRadius))
	if headingDegrees == nil {
		fillPolygon(canvas, colourMarkerOutline, circle(centre.x, centre.y, 5))
		return
	}
	// The device measures counter-clockwise with y up; the image has y down.
	angle := *headingDegrees * math.Pi / 180
	forwardX, forwardY := math.Cos(angle), -math.Sin(angle)
	sideX, sideY := -forwardY, forwardX
	at := func(forward, side float64) mapPosition {
		radius := float64(vacuumMarkerRadius)
		return mapPosition{
			x: centre.x + radius*(forward*forwardX+side*sideX),
			y: centre.y + radius*(forward*forwardY+side*sideY),
		}
	}
	fillPolygon(canvas, colourMarkerOutline, []mapPosition{at(0.72, 0), at(-0.4, 0.5), at(-0.18, 0), at(-0.4, -0.5)})
}

// strokePolyline draws a line of the given width through points, with round
// joins and ends.
func strokePolyline(canvas *image.NRGBA, stroke color.NRGBA, width float64, points []mapPosition) {
	radius := width / 2
	shapes := make([][]mapPosition, 0, 2*len(points))
	for index := 1; index < len(points); index++ {
		start, end := points[index-1], points[index]
		length := math.Hypot(end.x-start.x, end.y-start.y)
		if length == 0 {
			continue
		}
		normalX, normalY := -(end.y-start.y)/length*radius, (end.x-start.x)/length*radius
		shapes = append(shapes, []mapPosition{
			{start.x + normalX, start.y + normalY}, {end.x + normalX, end.y + normalY},
			{end.x - normalX, end.y - normalY}, {start.x - normalX, start.y - normalY},
		})
	}
	for _, point := range points {
		shapes = append(shapes, circle(point.x, point.y, radius))
	}
	fillShapes(canvas, stroke, shapes)
}

func fillPolygon(canvas *image.NRGBA, fill color.NRGBA, points []mapPosition) {
	fillShapes(canvas, fill, [][]mapPosition{points})
}

// fillShapes fills closed polygons in one rasterizer pass over their bounding
// box, so that where they overlap they do not build up opacity.
func fillShapes(canvas *image.NRGBA, fill color.NRGBA, shapes [][]mapPosition) {
	area := image.Rectangle{}
	for _, shape := range shapes {
		for _, point := range shape {
			area = area.Union(image.Rect(int(math.Floor(point.x)), int(math.Floor(point.y)), int(math.Ceil(point.x))+1, int(math.Ceil(point.y))+1))
		}
	}
	area = area.Intersect(canvas.Bounds())
	if area.Empty() {
		return
	}
	rasterizer := vector.NewRasterizer(area.Dx(), area.Dy())
	for _, shape := range shapes {
		addShape(rasterizer, shape, area.Min)
	}
	rasterizer.Draw(canvas, area, image.NewUniform(fill), image.Point{})
}

// addShape adds a closed polygon to a rasterizer whose top left is at origin,
// always wound the same way, so that overlapping shapes add up instead of
// cancelling out.
func addShape(rasterizer *vector.Rasterizer, points []mapPosition, origin image.Point) {
	signedArea := 0.0
	for index, point := range points {
		next := points[(index+1)%len(points)]
		signedArea += point.x*next.y - next.x*point.y
	}
	at := func(index int) (float32, float32) {
		if signedArea < 0 {
			index = len(points) - 1 - index
		}
		return float32(points[index].x - float64(origin.X)), float32(points[index].y - float64(origin.Y))
	}
	rasterizer.MoveTo(at(0))
	for index := 1; index < len(points); index++ {
		rasterizer.LineTo(at(index))
	}
	rasterizer.ClosePath()
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

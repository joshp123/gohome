package roborock

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"testing"
)

func appendMapBlock(payload []byte, blockType uint16, headerFields []uint32, data []byte) []byte {
	header := make([]byte, 8+4*len(headerFields))
	binary.LittleEndian.PutUint16(header[0:], blockType)
	binary.LittleEndian.PutUint16(header[2:], uint16(len(header)))
	binary.LittleEndian.PutUint32(header[4:], uint32(len(data)))
	for index, field := range headerFields {
		binary.LittleEndian.PutUint32(header[8+4*index:], field)
	}
	return append(append(payload, header...), data...)
}

func millimetrePoint(x, y uint32) []byte {
	point := make([]byte, 12)
	binary.LittleEndian.PutUint32(point[0:], x)
	binary.LittleEndian.PutUint32(point[4:], y)
	return point
}

// A small two-room map in the device's format: rooms 16 and 17, a wall
// between them, one scanned cell and one unassigned floor cell, which are
// not rooms.
func TestParseAndRenderVacuumMap(t *testing.T) {
	const width, height, left, top = 20, 10, 400, 500
	cells := make([]byte, width*height)
	for y := 1; y < height-1; y++ {
		for x := 1; x < width-1; x++ {
			cells[x+width*y] = 0x87 // room 16
			if x > width/2 {
				cells[x+width*y] = 0x8f // room 17
			}
		}
		cells[width/2+width*y] = 0x81 // wall of room 16
	}
	cells[1+width*1] = floorPixelScanned
	cells[2+width*1] = floorPixelUnassignedFloor

	payload := make([]byte, 20)
	binary.LittleEndian.PutUint16(payload[2:], 20)
	payload = appendMapBlock(payload, mapBlockFloor, []uint32{2, top, left, height, width}, cells)
	payload = appendMapBlock(payload, mapBlockDock, nil, millimetrePoint((left+3)*50, (top+2)*50))
	payload = appendMapBlock(payload, mapBlockVacuum, nil, millimetrePoint((left+15)*50, (top+7)*50))
	path := []byte{}
	for _, point := range [][2]uint16{{(left + 3) * 50, (top + 2) * 50}, {(left + 15) * 50, (top + 7) * 50}} {
		path = binary.LittleEndian.AppendUint16(path, point[0])
		path = binary.LittleEndian.AppendUint16(path, point[1])
	}
	payload = appendMapBlock(payload, mapBlockLastRunPath, []uint32{2, 4, 0}, path)

	parsed, err := parseVacuumMap(payload, map[uint32]string{16: "living_room"})
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.rooms) != 2 || parsed.rooms[0].id != 16 || parsed.rooms[1].id != 17 {
		t.Fatalf("rooms = %+v, want 16 and 17 only", parsed.rooms)
	}
	if parsed.rooms[0].label != "living_room" {
		t.Fatalf("room 16 label = %q", parsed.rooms[0].label)
	}
	// Device row 2 from the south is image row height-1-2.
	if parsed.dock == nil || parsed.dock.x != 3 || parsed.dock.y != height-1-2 {
		t.Fatalf("dock = %+v", parsed.dock)
	}
	if parsed.vacuum == nil || parsed.vacuum.x != 15 || parsed.vacuum.y != height-1-7 {
		t.Fatalf("vacuum = %+v", parsed.vacuum)
	}
	if len(parsed.lastRunPath) != 2 || parsed.lastRunPath[1] != *parsed.vacuum {
		t.Fatalf("last run path = %+v", parsed.lastRunPath)
	}

	for _, labelMode := range []string{"names", "segments", ""} {
		encoded, err := renderVacuumMap(parsed, labelMode, true)
		if err != nil {
			t.Fatal(err)
		}
		image, err := png.Decode(bytes.NewReader(encoded))
		if err != nil {
			t.Fatal(err)
		}
		wantWidth := (width - 2 + 2*cropMarginCells) * outputPixelsPerCell
		if image.Bounds().Dx() != wantWidth {
			t.Fatalf("labels=%q width = %d, want %d", labelMode, image.Bounds().Dx(), wantWidth)
		}
	}
}

// One room in which the user erased an area, laid a carpet, drew a no-go
// zone and a virtual wall, with the vacuum facing up the map.
func TestErasedAreasZonesAndHeading(t *testing.T) {
	const width, height, left, top = 24, 14, 400, 500
	cells := make([]byte, width*height)
	submap := make([]byte, width*height)
	for y := 1; y < height-1; y++ {
		for x := 1; x < width-1; x++ {
			cells[x+width*y] = 0x87 // room 16
		}
	}
	for y := 3; y <= 5; y++ {
		for x := 3; x <= 5; x++ {
			submap[x+width*y] = submapRemovedArea
		}
		submap[2+width*y] = submapRemovedBorder | submapCarpet
	}
	submap[16+width*9] = submapCarpet

	millimetres := func(cellX, cellY int) []byte {
		point := binary.LittleEndian.AppendUint16(nil, uint16((left+cellX)*50))
		return binary.LittleEndian.AppendUint16(point, uint16((top+cellY)*50))
	}
	var noGoArea []byte
	for _, corner := range [][2]int{{14, 2}, {18, 2}, {18, 5}, {14, 5}} {
		noGoArea = append(noGoArea, millimetres(corner[0], corner[1])...)
	}
	vacuum := millimetrePoint((left+10)*50, (top+7)*50)
	binary.LittleEndian.PutUint32(vacuum[8:], 90)

	payload := make([]byte, 20)
	binary.LittleEndian.PutUint16(payload[2:], 20)
	payload = appendMapBlock(payload, mapBlockFloor, []uint32{2, top, left, height, width}, cells)
	payload = appendMapBlock(payload, mapBlockVacuum, nil, vacuum)
	payload = appendMapBlock(payload, mapBlockNoGoAreas, []uint32{1}, noGoArea)
	payload = appendMapBlock(payload, mapBlockVirtualWalls, []uint32{1}, append(millimetres(8, 2), millimetres(8, 10)...))
	payload = appendMapBlock(payload, mapBlockSubmap, nil, submap)

	parsed, err := parseVacuumMap(payload, nil)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.vacuumHeadingDegrees == nil || *parsed.vacuumHeadingDegrees != 90 {
		t.Fatalf("heading = %v, want 90", parsed.vacuumHeadingDegrees)
	}
	if len(parsed.noGoAreas) != 1 || parsed.noGoAreas[0][2] != parsed.floor.imagePosition((left+18)*50, (top+5)*50) {
		t.Fatalf("no-go areas = %+v", parsed.noGoAreas)
	}
	if len(parsed.virtualWalls) != 1 || parsed.virtualWalls[0][1] != parsed.floor.imagePosition((left+8)*50, (top+10)*50) {
		t.Fatalf("virtual walls = %+v", parsed.virtualWalls)
	}
	if !parsed.carpetCells[16+width*9] || parsed.carpetCells[2+width*4] {
		t.Fatal("carpet must come from bit 0x01 of cells that were not erased")
	}
	if parsed.rooms[0].pixelCount != (width-2)*(height-2)-9-3 {
		t.Fatalf("room 16 has %d cells; erased cells must not count", parsed.rooms[0].pixelCount)
	}

	render := func(vacuumMapData vacuumMap) image.Image {
		encoded, err := renderVacuumMap(vacuumMapData, "", false)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := png.Decode(bytes.NewReader(encoded))
		if err != nil {
			t.Fatal(err)
		}
		return decoded
	}
	crop, _ := floorPlanBounds(parsed)
	outputPixel := func(cellX, deviceRow int) (int, int) {
		return (cellX-crop.Min.X)*outputPixelsPerCell + outputPixelsPerCell/2,
			(height-1-deviceRow-crop.Min.Y)*outputPixelsPerCell + outputPixelsPerCell/2
	}
	withZones := render(parsed)
	if _, _, _, alpha := withZones.At(outputPixel(4, 4)).RGBA(); alpha != 0 {
		t.Fatalf("erased area drawn with alpha %d, want transparent", alpha)
	}
	if _, _, _, alpha := withZones.At(outputPixel(12, 11)).RGBA(); alpha != 0xffff {
		t.Fatalf("room drawn with alpha %d, want opaque", alpha)
	}
	withoutZones := parsed
	withoutZones.noGoAreas = nil
	if withZones.At(outputPixel(16, 3)) == render(withoutZones).At(outputPixel(16, 3)) {
		t.Fatal("no-go zone not drawn")
	}
}

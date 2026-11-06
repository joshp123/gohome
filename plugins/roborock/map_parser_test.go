package roborock

import (
	"bytes"
	"encoding/binary"
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

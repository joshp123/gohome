package roborock

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"math"
	"sort"
)

// The Roborock map is a header followed by typed blocks. Block numbers and
// pixel meanings follow vacuum-map-parser-roborock, the parser Home Assistant
// uses. Positions are in millimetres; the floor grid has one cell per 50 mm.
const (
	mapBlockDock           = 1
	mapBlockFloor          = 2
	mapBlockLastRunPath    = 3
	mapBlockVacuum         = 8
	mapBlockCarpet         = 17
	millimetresPerMapPixel = 50
)

const (
	floorPixelOutside         = 0x00
	floorPixelWall            = 0x01
	floorPixelScanned         = 0x07
	floorPixelUnassignedFloor = 0xFF
)

type MapBlockInfo struct {
	Type   int
	Length int
}

// floorGrid holds the device's floor cells. The device stores the southern
// row first; pixelAt takes image coordinates, where y grows downwards.
type floorGrid struct {
	left   int
	top    int
	width  int
	height int
	cells  []byte
}

func (g floorGrid) cellIndex(x, y int) int {
	return x + g.width*(g.height-1-y)
}

func (g floorGrid) pixelAt(x, y int) byte {
	return g.cells[g.cellIndex(x, y)]
}

// imagePosition converts a device position in millimetres to floor-grid image coordinates.
func (g floorGrid) imagePosition(xMillimetres, yMillimetres float64) mapPosition {
	return mapPosition{
		x: xMillimetres/millimetresPerMapPixel - float64(g.left),
		y: float64(g.height-1) - (yMillimetres/millimetresPerMapPixel - float64(g.top)),
	}
}

// mapPosition is a point in floor-grid image coordinates, in cells.
type mapPosition struct {
	x float64
	y float64
}

type vacuumMap struct {
	floor       floorGrid
	carpetCells map[int]bool
	dock        *mapPosition
	vacuum      *mapPosition
	lastRunPath []mapPosition
	rooms       []segmentSummary
}

// segmentSummary describes one room (segment) in floor-grid image coordinates.
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

// roomIDForPixel reports the room a floor cell belongs to. Room floor cells
// carry the room id in their top five bits and 0b111 in the low three;
// 0x07 (scanned, no room) and 0xFF (floor outside any room) carry no room.
func roomIDForPixel(pixel byte) (int, bool) {
	if pixel&0x07 != 0x07 || pixel == floorPixelScanned || pixel == floorPixelUnassignedFloor {
		return 0, false
	}
	return int(pixel >> 3), true
}

// roomIDForWallOrObstacle reports the room a wall or obstacle cell borders.
// These cells keep the room id in their top five bits and use low bits 0b000
// (obstacle) or 0b001 (wall).
func roomIDForWallOrObstacle(pixel byte) (int, bool) {
	if pixel>>3 == 0 || (pixel&0x07 != 0x00 && pixel&0x07 != 0x01) {
		return 0, false
	}
	return int(pixel >> 3), true
}

type mapBlock struct {
	blockType int
	header    []byte
	data      []byte
}

func decompressMap(raw []byte) ([]byte, error) {
	if len(raw) >= 2 && raw[0] == 0x1f && raw[1] == 0x8b {
		reader, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		return io.ReadAll(reader)
	}
	return raw, nil
}

func readMapBlocks(raw []byte) ([]mapBlock, error) {
	data, err := decompressMap(raw)
	if err != nil {
		return nil, err
	}
	if len(data) < 4 {
		return nil, fmt.Errorf("map payload too short")
	}
	mapHeaderLength := int(uint16le(data, 0x02))
	if mapHeaderLength <= 0 || mapHeaderLength >= len(data) {
		return nil, fmt.Errorf("invalid map header length")
	}
	var blocks []mapBlock
	blockStart := mapHeaderLength
	for blockStart+8 <= len(data) {
		blockHeaderLength := int(uint16le(data, blockStart+0x02))
		if blockHeaderLength < 8 || blockStart+blockHeaderLength > len(data) {
			break
		}
		header := data[blockStart : blockStart+blockHeaderLength]
		blockDataLength := int(uint32le(header, 0x04))
		blockDataStart := blockStart + blockHeaderLength
		if blockDataLength < 0 || blockDataStart+blockDataLength > len(data) {
			break
		}
		blocks = append(blocks, mapBlock{
			blockType: int(uint16le(header, 0x00)),
			header:    header,
			data:      data[blockDataStart : blockDataStart+blockDataLength],
		})
		blockStart = blockDataStart + blockDataLength
	}
	return blocks, nil
}

func ListMapBlocks(raw []byte) ([]MapBlockInfo, error) {
	blocks, err := readMapBlocks(raw)
	if err != nil {
		return nil, err
	}
	infos := make([]MapBlockInfo, 0, len(blocks))
	for _, block := range blocks {
		infos = append(infos, MapBlockInfo{Type: block.blockType, Length: len(block.data)})
	}
	return infos, nil
}

func parseVacuumMap(raw []byte, roomNames map[uint32]string) (vacuumMap, error) {
	blocks, err := readMapBlocks(raw)
	if err != nil {
		return vacuumMap{}, err
	}
	var parsed vacuumMap
	foundFloor := false
	for _, block := range blocks {
		if block.blockType != mapBlockFloor || len(block.header) < 24 {
			continue
		}
		// The floor block header ends with top, left, height and width.
		end := len(block.header)
		grid := floorGrid{
			top:    int(int32(uint32le(block.header, end-16))),
			left:   int(int32(uint32le(block.header, end-12))),
			height: int(int32(uint32le(block.header, end-8))),
			width:  int(int32(uint32le(block.header, end-4))),
		}
		if grid.width <= 0 || grid.height <= 0 || len(block.data) < grid.width*grid.height {
			return vacuumMap{}, fmt.Errorf("invalid map floor block")
		}
		grid.cells = block.data[:grid.width*grid.height]
		parsed.floor = grid
		foundFloor = true
	}
	if !foundFloor {
		return vacuumMap{}, fmt.Errorf("map floor block not found")
	}
	for _, block := range blocks {
		switch block.blockType {
		case mapBlockDock:
			parsed.dock = parseMapPosition(parsed.floor, block.data)
		case mapBlockVacuum:
			parsed.vacuum = parseMapPosition(parsed.floor, block.data)
		case mapBlockLastRunPath:
			for offset := 0; offset+4 <= len(block.data); offset += 4 {
				parsed.lastRunPath = append(parsed.lastRunPath, parsed.floor.imagePosition(
					float64(uint16le(block.data, offset)),
					float64(uint16le(block.data, offset+2)),
				))
			}
		case mapBlockCarpet:
			parsed.carpetCells = make(map[int]bool)
			for index, value := range block.data {
				if value != 0 {
					parsed.carpetCells[index] = true
				}
			}
		}
	}
	parsed.rooms = summariseRooms(parsed.floor, roomNames)
	return parsed, nil
}

func parseMapPosition(grid floorGrid, data []byte) *mapPosition {
	if len(data) < 8 {
		return nil
	}
	position := grid.imagePosition(float64(int32(uint32le(data, 0))), float64(int32(uint32le(data, 4))))
	return &position
}

func summariseRooms(grid floorGrid, roomNames map[uint32]string) []segmentSummary {
	rooms := make(map[int]*segmentSummary)
	for y := 0; y < grid.height; y++ {
		for x := 0; x < grid.width; x++ {
			id, ok := roomIDForPixel(grid.pixelAt(x, y))
			if !ok {
				continue
			}
			room, seen := rooms[id]
			if !seen {
				room = &segmentSummary{id: id, minX: x, maxX: x, minY: y, maxY: y, label: roomNames[uint32(id)]}
				rooms[id] = room
			}
			room.pixelCount++
			room.sumX += x
			room.sumY += y
			room.minX = min(room.minX, x)
			room.maxX = max(room.maxX, x)
			room.minY = min(room.minY, y)
			room.maxY = max(room.maxY, y)
		}
	}
	result := make([]segmentSummary, 0, len(rooms))
	for _, room := range rooms {
		result = append(result, *room)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].id < result[j].id })
	return result
}

func uint16le(data []byte, offset int) uint16 {
	return uint16(data[offset]) | uint16(data[offset+1])<<8
}

func uint32le(data []byte, offset int) uint32 {
	return uint32(data[offset]) | uint32(data[offset+1])<<8 | uint32(data[offset+2])<<16 | uint32(data[offset+3])<<24
}

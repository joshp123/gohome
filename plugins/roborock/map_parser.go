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
	mapBlockNoGoAreas      = 9
	mapBlockVirtualWalls   = 10
	mapBlockNoMopAreas     = 12
	mapBlockSubmap         = 17
	millimetresPerMapPixel = 50
)

// Each submap byte is a bitfield over the floor cell with the same index.
// Removed cells are areas the user erased in the Roborock app.
const (
	submapCarpet        = 0x01
	submapRemovedBorder = 0x08
	submapRemovedArea   = 0x10
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

// mapArea is a four-cornered zone the user drew in the app.
type mapArea [4]mapPosition

// virtualWall is a line the user drew in the app that the vacuum must not cross.
type virtualWall [2]mapPosition

type vacuumMap struct {
	floor       floorGrid
	carpetCells map[int]bool
	dock        *mapPosition
	vacuum      *mapPosition
	// vacuumHeadingDegrees is where the vacuum faces, counter-clockwise from
	// the map's rightward direction; nil when the device does not report it.
	vacuumHeadingDegrees *float64
	lastRunPath          []mapPosition
	noGoAreas            []mapArea
	noMopAreas           []mapArea
	virtualWalls         []virtualWall
	rooms                []segmentSummary
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
			parsed.vacuumHeadingDegrees = parseHeadingDegrees(block.data)
		case mapBlockLastRunPath:
			parsed.lastRunPath = parseMillimetrePoints(parsed.floor, block.data)
		case mapBlockNoGoAreas:
			parsed.noGoAreas = parseAreas(parsed.floor, block)
		case mapBlockNoMopAreas:
			parsed.noMopAreas = parseAreas(parsed.floor, block)
		case mapBlockVirtualWalls:
			parsed.virtualWalls = parseVirtualWalls(parsed.floor, block)
		case mapBlockSubmap:
			parsed.carpetCells, parsed.floor.cells = applySubmap(parsed.floor.cells, block.data)
		}
	}
	parsed.rooms = summariseRooms(parsed.floor, roomNames)
	return parsed, nil
}

// applySubmap returns the carpet cells and a copy of the floor cells with
// erased areas turned into outside, so that cropping, rooms and labels all
// ignore them.
func applySubmap(floorCells, submap []byte) (map[int]bool, []byte) {
	cells := append([]byte(nil), floorCells...)
	carpetCells := make(map[int]bool)
	for index, value := range submap {
		if index >= len(cells) {
			break
		}
		if value&(submapRemovedBorder|submapRemovedArea) != 0 {
			cells[index] = floorPixelOutside
			continue
		}
		if value&submapCarpet != 0 {
			carpetCells[index] = true
		}
	}
	return carpetCells, cells
}

func parseMapPosition(grid floorGrid, data []byte) *mapPosition {
	if len(data) < 8 {
		return nil
	}
	position := grid.imagePosition(float64(int32(uint32le(data, 0))), float64(int32(uint32le(data, 4))))
	return &position
}

// parseHeadingDegrees reads the signed heading that follows a position.
func parseHeadingDegrees(data []byte) *float64 {
	if len(data) < 12 {
		return nil
	}
	heading := float64(int32(uint32le(data, 8)))
	return &heading
}

// parseMillimetrePoints reads consecutive (x, y) pairs of unsigned 16-bit millimetres.
func parseMillimetrePoints(grid floorGrid, data []byte) []mapPosition {
	points := make([]mapPosition, 0, len(data)/4)
	for offset := 0; offset+4 <= len(data); offset += 4 {
		points = append(points, grid.imagePosition(float64(uint16le(data, offset)), float64(uint16le(data, offset+2))))
	}
	return points
}

// zoneCount is the number of zones or walls a block says it holds.
func zoneCount(block mapBlock) int {
	if len(block.header) < 0x0a {
		return 0
	}
	return int(uint16le(block.header, 0x08))
}

func parseAreas(grid floorGrid, block mapBlock) []mapArea {
	const areaBytes = 16
	count := min(zoneCount(block), len(block.data)/areaBytes)
	areas := make([]mapArea, 0, count)
	for index := 0; index < count; index++ {
		corners := parseMillimetrePoints(grid, block.data[index*areaBytes:(index+1)*areaBytes])
		areas = append(areas, mapArea{corners[0], corners[1], corners[2], corners[3]})
	}
	return areas
}

func parseVirtualWalls(grid floorGrid, block mapBlock) []virtualWall {
	const wallBytes = 8
	count := min(zoneCount(block), len(block.data)/wallBytes)
	walls := make([]virtualWall, 0, count)
	for index := 0; index < count; index++ {
		ends := parseMillimetrePoints(grid, block.data[index*wallBytes:(index+1)*wallBytes])
		walls = append(walls, virtualWall{ends[0], ends[1]})
	}
	return walls
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

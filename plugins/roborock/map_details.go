package roborock

import (
	"context"
	"fmt"
	roborockv1 "github.com/joshp123/gohome/proto/gen/plugins/roborock/v1"
	"strconv"
	"strings"
)

func (s *service) GetMapDetails(ctx context.Context, request *roborockv1.MapDetailsRequest) (*roborockv1.MapDetailsResponse, error) {
	id, err := s.requireDevice(ctx, request.GetDeviceId())
	if err != nil {
		return nil, err
	}
	if _, err := s.client.currentMap(ctx, id); err != nil {
		return nil, mapClientError("read map", err)
	}
	s.client.mu.Lock()
	raw := s.client.mapCache[id].raw
	roomNames := map[string]string{}
	for _, r := range s.client.homeData.Rooms {
		roomNames[strconv.FormatInt(r.ID, 10)] = r.Name
	}
	s.client.mu.Unlock()
	response := &roborockv1.MapDetailsResponse{RawMap: raw}
	response.Obstacles, err = mapObstacles(raw)
	if err != nil {
		return nil, mapClientError("decode obstacles", err)
	}
	device, err := s.client.deviceByID(id)
	if err != nil {
		return nil, err
	}
	result, err := s.client.readRPC(ctx, device, "get_room_mapping", nil)
	if err != nil {
		return nil, mapClientError("read room mapping", err)
	}
	entries, ok := result.([]any)
	if !ok {
		return nil, fmt.Errorf("invalid room mapping")
	}
	for _, entry := range entries {
		fields, ok := entry.([]any)
		if !ok || len(fields) < 2 {
			return nil, fmt.Errorf("invalid room mapping entry")
		}
		room := &roborockv1.RoomMapping{SegmentId: int64(intFrom(fields[0])), CloudRoomId: stringFrom(fields[1])}
		room.Name = roomNames[room.CloudRoomId]
		if room.Name == "" {
			room.Name = s.client.cfg.SegmentNames[uint32(room.SegmentId)]
		}
		if len(fields) > 2 {
			v := int64(intFrom(fields[2]))
			room.TypeCode = &v
		}
		response.Rooms = append(response.Rooms, room)
	}
	return response, nil
}

func mapObstacles(raw []byte) ([]*roborockv1.MapObstacle, error) {
	blocks, err := readMapBlocks(raw)
	if err != nil {
		return nil, err
	}
	out := []*roborockv1.MapObstacle{}
	for _, block := range blocks {
		if block.blockType < 13 || block.blockType > 16 {
			continue
		}
		if len(block.header) < 12 {
			return nil, fmt.Errorf("short obstacle header")
		}
		count := int(uint16le(block.header, 8))
		if count == 0 {
			continue
		}
		if len(block.data)%count != 0 || len(block.data)/count < 4 {
			return nil, fmt.Errorf("invalid obstacle size")
		}
		size := len(block.data) / count
		for offset := 0; offset < len(block.data); offset += size {
			data := block.data[offset : offset+size]
			o := &roborockv1.MapObstacle{XMillimetres: int64(uint16le(data, 0)), YMillimetres: int64(uint16le(data, 2)), Ignored: block.blockType == 14 || block.blockType == 16}
			if size >= 6 {
				v := int64(uint16le(data, 4))
				o.TypeCode = &v
			}
			if size == 28 {
				o.PhotoId = strings.TrimRight(string(data[12:28]), "\x00")
			}
			out = append(out, o)
		}
	}
	return out, nil
}

func (s *service) ListSavedMaps(ctx context.Context, request *roborockv1.DeviceStatusRequest) (*roborockv1.SavedMapsResponse, error) {
	id, err := s.requireDevice(ctx, request.GetDeviceId())
	if err != nil {
		return nil, err
	}
	d, err := s.client.deviceByID(id)
	if err != nil {
		return nil, err
	}
	raw, err := s.client.readRPC(ctx, d, "get_multi_maps_list", nil)
	if err != nil {
		return nil, mapClientError("read map list", err)
	}
	values, ok := normalizeMap(raw)
	if !ok {
		return nil, fmt.Errorf("invalid map list")
	}
	var data struct {
		Maps []savedMap `json:"map_info"`
	}
	if err := decodeDeviceObject(values, &data); err != nil {
		return nil, err
	}
	response := &roborockv1.SavedMapsResponse{}
	for _, m := range data.Maps {
		response.Maps = append(response.Maps, m.proto())
	}
	return response, nil
}

type savedMap struct {
	ID      int64      `json:"mapFlag"`
	Name    string     `json:"name"`
	Created int64      `json:"add_time"`
	Backups []savedMap `json:"bak_maps"`
}

func (m savedMap) proto() *roborockv1.SavedMap {
	r := &roborockv1.SavedMap{MapId: m.ID, Name: m.Name, CreatedTimestampSeconds: m.Created}
	for _, b := range m.Backups {
		r.Backups = append(r.Backups, b.proto())
	}
	return r
}

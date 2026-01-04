package roborock

import (
	"encoding/json"
	"errors"
	"fmt"
)

type requestMessage struct {
	Method    string `json:"method"`
	Params    any    `json:"params"`
	RequestID int    `json:"id"`
	Timestamp uint32 `json:"-"`
	Security  any    `json:"security,omitempty"`
}

type rpcResponse struct {
	RequestID int `json:"id"`
	Result    any `json:"result"`
	Error     any `json:"error"`
}

func encodeRequestPayload(req requestMessage) ([]byte, error) {
	innerBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	outer := map[string]any{
		"dps": map[string]string{
			"101": string(innerBytes),
		},
		"t": req.Timestamp,
	}
	return json.Marshal(outer)
}

func decodeResponsePayload(payload []byte) (rpcResponse, error) {
	if len(payload) == 0 {
		return rpcResponse{}, errors.New("empty payload")
	}
	var outer map[string]any
	if err := json.Unmarshal(payload, &outer); err != nil {
		return rpcResponse{}, err
	}
	dps, ok := outer["dps"].(map[string]any)
	if !ok {
		if _, hasResult := outer["result"]; hasResult {
			raw, err := json.Marshal(outer)
			if err != nil {
				return rpcResponse{}, err
			}
			var resp rpcResponse
			if err := json.Unmarshal(raw, &resp); err != nil {
				return rpcResponse{}, err
			}
			return resp, nil
		}
		return rpcResponse{}, fmt.Errorf("invalid dps in payload")
	}
	val, ok := dps["102"]
	if !ok {
		if alt, okAlt := dps["101"]; okAlt {
			val = alt
			ok = true
		} else if len(dps) == 1 {
			for _, v := range dps {
				val = v
				ok = true
				break
			}
		}
	}
	if !ok {
		return rpcResponse{}, fmt.Errorf("missing dps 102")
	}

	switch typed := val.(type) {
	case string:
		var resp rpcResponse
		if err := json.Unmarshal([]byte(typed), &resp); err != nil {
			return rpcResponse{}, err
		}
		return resp, nil
	case map[string]any:
		raw, err := json.Marshal(typed)
		if err != nil {
			return rpcResponse{}, err
		}
		var resp rpcResponse
		if err := json.Unmarshal(raw, &resp); err != nil {
			return rpcResponse{}, err
		}
		return resp, nil
	default:
		return rpcResponse{}, fmt.Errorf("invalid dps response type %T", val)
	}
}

package stillroom

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Client event types.
const (
	EventSit    = "sit"
	EventRise   = "rise"
	EventBreath = "breath"
)

// Server message types.
const (
	MsgPresence       = "presence"
	MsgMandala        = "mandala"
	MsgBell           = "bell"
	MsgSilenceBroken  = "silence_broken"
	MsgError          = "error"
	BellReasonAligned = "breath_alignment"
)

var (
	ErrInvalidJSON   = errors.New("invalid json")
	ErrUnknownType   = errors.New("unknown type")
	ErrExtraFields   = errors.New("extra fields")
	ErrInvalidPhase  = errors.New("invalid phase")
	ErrMissingPhase  = errors.New("missing phase")
)

// ClientEvent is a validated client message.
type ClientEvent struct {
	Type  string
	Phase float64
}

// ParseClientMessage validates inbound JSON from clients.
func ParseClientMessage(data []byte) (ClientEvent, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return ClientEvent{}, ErrInvalidJSON
	}

	typeField, ok := raw["type"]
	if !ok {
		return ClientEvent{}, ErrUnknownType
	}

	var typeStr string
	if err := json.Unmarshal(typeField, &typeStr); err != nil {
		return ClientEvent{}, ErrUnknownType
	}

	switch typeStr {
	case EventSit:
		if len(raw) != 1 {
			return ClientEvent{}, ErrExtraFields
		}
		return ClientEvent{Type: EventSit}, nil

	case EventRise:
		if len(raw) != 1 {
			return ClientEvent{}, ErrExtraFields
		}
		return ClientEvent{Type: EventRise}, nil

	case EventBreath:
		if len(raw) != 2 {
			return ClientEvent{}, ErrExtraFields
		}
		phaseRaw, ok := raw["phase"]
		if !ok {
			return ClientEvent{}, ErrMissingPhase
		}
		var phase float64
		if err := json.Unmarshal(phaseRaw, &phase); err != nil {
			return ClientEvent{}, ErrInvalidPhase
		}
		if phase < 0 || phase > 1 {
			return ClientEvent{}, ErrInvalidPhase
		}
		return ClientEvent{Type: EventBreath, Phase: phase}, nil

	default:
		return ClientEvent{}, ErrUnknownType
	}
}

func encodePresence(sitting int, stillnessSeconds uint64) []byte {
	b, _ := json.Marshal(map[string]any{
		"type":              MsgPresence,
		"sitting":           sitting,
		"stillness_seconds": stillnessSeconds,
	})
	return b
}

func encodeMandala(seedHex string, layers int) []byte {
	b, _ := json.Marshal(map[string]any{
		"type":   MsgMandala,
		"seed":   seedHex,
		"layers": layers,
	})
	return b
}

func encodeBell() []byte {
	b, _ := json.Marshal(map[string]any{
		"type":   MsgBell,
		"reason": BellReasonAligned,
	})
	return b
}

func encodeSilenceBroken(message string) []byte {
	b, _ := json.Marshal(map[string]any{
		"type":    MsgSilenceBroken,
		"message": message,
	})
	return b
}

// MandalaLayers returns layer count from stillness seconds.
func MandalaLayers(stillnessSeconds uint64) int {
	layers := int(stillnessSeconds / 60)
	if layers > 12 {
		return 12
	}
	return layers
}

// PhaseDistance returns circular distance between two breath phases in [0,1].
func PhaseDistance(a, b float64) float64 {
	d := a - b
	if d < 0 {
		d = -d
	}
	if d > 0.5 {
		d = 1 - d
	}
	return d
}

// FormatSeed returns hex encoding of seed bytes.
func FormatSeed(seed []byte) string {
	return fmt.Sprintf("%x", seed)
}

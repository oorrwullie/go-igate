package aprs

import (
	"fmt"
	"strings"
	"time"
)

type Packet struct {
	Src       string
	Dst       string
	Path      []string
	Payload   string
	Timestamp time.Time
}

type PacketType int

const (
	Unknown PacketType = iota
	PositionReport
	StatusReport
	Message
	Telemetry
	WeatherReport
	ObjectReport
	ItemReport
	Query
	ThirdPartyTraffic
	RawGPSData
)

func ParsePacket(p string) (*Packet, error) {
	parts := strings.Split(p, ">")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid packet format")
	}

	src := parts[0]
	rest := parts[1]
	pathInfo := strings.SplitN(rest, ":", 2)
	if len(pathInfo) < 2 {
		return nil, fmt.Errorf("invalid packet format")
	}

	path := strings.Split(pathInfo[0], ",")
	dst := path[0]
	path = path[1:]
	payload := pathInfo[1]

	return &Packet{
		Src:     src,
		Dst:     dst,
		Path:    path,
		Payload: payload,
	}, nil
}

func (p *Packet) Type() PacketType {
	if len(p.Payload) == 0 {
		return Unknown
	}

	typeChar := p.Payload[0]

	if strings.ContainsRune("!=/@", rune(typeChar)) || (typeChar == ':' && len(p.Payload) > 1 && strings.ContainsRune("!'#/)", rune(p.Payload[1]))) {
		return PositionReport
	}

	if typeChar == ':' && len(p.Payload) > 9 && p.Payload[9] == ':' {
		return Message
	}

	fmt.Printf("Type char: %v\n", p.Payload[0])

	switch p.Payload[0] {
	case '>':
		return StatusReport
	case 'T':
		return Telemetry
	case '_':
		return WeatherReport
	case ';':
		return ObjectReport
	case ')':
		return ItemReport
	case '?':
		return Query
	case '{':
		return ThirdPartyTraffic
	case '$':
		return RawGPSData
	default:
		return Unknown
	}
}

// AckString returns a string representation of the packet for sending an acknowledgement
func (p *Packet) AckString() (string, error) {
	if len(p.Payload) < 4 || p.Payload[len(p.Payload)-4] != '{' {
		return "", fmt.Errorf("No valid ack number found in the message")
	}

	ackNumber := p.Payload[len(p.Payload)-3:]

	ack := fmt.Sprintf(
		"%s>APRS,%s::%s:ack%s",
		p.Dst,
		strings.Join(p.Path, ","),
		p.Src,
		ackNumber,
	)

	return ack, nil
}

func (p *Packet) IsAckMessage() bool {
	if len(p.Payload) >= 6 && strings.HasPrefix(p.Payload, ":") {
		body := strings.TrimSpace(p.Payload[1:])

		return strings.HasPrefix(body, "ack") && len(body) == 6
	}

	return false
}

func (t PacketType) String() string {
	return [...]string{
		"Unknown", "Position Report", "Status Report", "Message",
		"Telemetry", "Weather Report", "Object Report", "Item Report",
		"Query", "Third Party Traffic", "Raw GPS Data"}[t]
}

// Forward determines if a packet should be forwarded to APRS-IS
func (t PacketType) ForwardToAprsIs() bool {
	switch t {
	case PositionReport, StatusReport, Message,
		WeatherReport, ObjectReport, ItemReport:
		return true
	default:
		return false
	}
}

func (t PacketType) NeedsAck() bool {
	switch t {
	case Message:
		return true
	default:
		return false
	}
}

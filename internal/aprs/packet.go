package aprs

import (
	"crypto/sha1"
	"fmt"
	"strconv"
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
	p = strings.TrimRight(p, "\r\n")
	parts := strings.SplitN(p, ">", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid packet format")
	}

	src := strings.TrimSpace(parts[0])
	if src == "" {
		return nil, fmt.Errorf("invalid packet format: missing source")
	}
	rest := parts[1]
	pathInfo := strings.SplitN(rest, ":", 2)
	if len(pathInfo) < 2 {
		return nil, fmt.Errorf("invalid packet format")
	}

	path := strings.Split(pathInfo[0], ",")
	dst := strings.TrimSpace(path[0])
	if dst == "" {
		return nil, fmt.Errorf("invalid packet format: missing destination")
	}
	path = path[1:]
	payload := pathInfo[1]

	return &Packet{
		Src:     src,
		Dst:     dst,
		Path:    path,
		Payload: payload,
	}, nil
}

// HasForbiddenRFPath reports path markers that identify packets which must not
// be retransmitted from RF or gated back to APRS-IS. q-constructs are an
// APRS-IS-only mechanism and must never be placed on RF.
func (p *Packet) HasForbiddenRFPath() bool {
	for _, component := range p.Path {
		component = strings.TrimSuffix(strings.ToUpper(strings.TrimSpace(component)), "*")
		switch {
		case component == "NOGATE", component == "RFONLY":
			return true
		case strings.HasPrefix(component, "TCPIP"), strings.HasPrefix(component, "TCPXX"):
			return true
		case strings.HasPrefix(component, "Q"):
			return true
		}
	}

	return false
}

// UnwrapThirdPartyForAprsIs removes the third-party data type and RF wrapper
// when the enclosed packet did not originate on APRS-IS. Packets carrying
// TCPIP/TCPXX in the enclosed path originated on APRS-IS and must not be
// injected back into APRS-IS.
func (p *Packet) UnwrapThirdPartyForAprsIs() (*Packet, bool) {
	if p.Type() != ThirdPartyTraffic {
		return p, true
	}

	inner, err := ParsePacket(strings.TrimPrefix(p.Payload, "}"))
	if err != nil || inner.HasForbiddenRFPath() {
		return nil, false
	}

	return inner, true
}

func (p *Packet) Type() PacketType {
	if len(p.Payload) == 0 {
		return Unknown
	}

	typeChar := p.Payload[0]

	if strings.ContainsRune("!=/@`", rune(typeChar)) || (typeChar == ':' && len(p.Payload) > 1 && strings.ContainsRune("!'#/)", rune(p.Payload[1]))) {
		return PositionReport
	}

	if typeChar == ':' && len(p.Payload) > 10 && p.Payload[10] == ':' {
		return Message
	}

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
	case '}':
		return ThirdPartyTraffic
	case '$':
		return RawGPSData
	default:
		return Unknown
	}
}

// MessageDestination returns the padded nine-character APRS message
// addressee, normalized for callsign comparisons.
func (p *Packet) MessageDestination() (string, bool) {
	if p.Type() != Message || len(p.Payload) < 11 {
		return "", false
	}

	return strings.ToUpper(strings.TrimSpace(p.Payload[1:10])), true
}

// AckString returns a string representation of the packet for sending an acknowledgement
func (p *Packet) AckString(stationCallsign string) string {
	ackNumber := p.generateAckNumber()

	return fmt.Sprintf(
		"%s>APRS:%s:ack%s",
		stationCallsign,
		p.Dst,
		ackNumber,
	)
}

func (p *Packet) generateAckNumber() string {
	packetString := fmt.Sprintf(
		"%s>%s:%s",
		p.Dst,
		strings.Join(p.Path, ","),
		p.Src,
	)

	hash := sha1.New()
	hash.Write([]byte(packetString))
	hashSum := hash.Sum(nil)

	return fmt.Sprintf("%03d", int(hashSum[0])%1000)
}

func (p *Packet) IsAckMessage() bool {
	if len(p.Payload) >= 6 && strings.HasPrefix(p.Payload, ":") {
		body := strings.TrimSpace(p.Payload[1:])

		return strings.HasPrefix(body, "ack") && len(body) == 6
	}

	return false
}

// Position returns latitude and longitude in decimal degrees if the payload contains an
// uncompressed position report (e.g. !DDMM.mmN/DDDMM.mmE). Otherwise ok will be false.
func (p *Packet) Position() (lat float64, lon float64, ok bool) {
	if len(p.Payload) < 19 {
		return 0, 0, false
	}

	switch p.Payload[0] {
	case '!', '=', '/', '@':
	default:
		return 0, 0, false
	}

	if len(p.Payload) < 19 {
		return 0, 0, false
	}

	latField := p.Payload[1:9] // DDMM.mmN
	if len(latField) != 8 {
		return 0, 0, false
	}

	latDeg, err := strconv.ParseFloat(latField[:2], 64)
	if err != nil {
		return 0, 0, false
	}

	latMin, err := strconv.ParseFloat(latField[2:7], 64)
	if err != nil {
		return 0, 0, false
	}

	latDir := latField[7]

	lonField := p.Payload[10:19] // DDDMM.mmE
	if len(lonField) != 9 {
		return 0, 0, false
	}

	lonDeg, err := strconv.ParseFloat(lonField[:3], 64)
	if err != nil {
		return 0, 0, false
	}

	lonMin, err := strconv.ParseFloat(lonField[3:8], 64)
	if err != nil {
		return 0, 0, false
	}

	lonDir := lonField[8]

	lat = latDeg + latMin/60
	lon = lonDeg + lonMin/60

	switch latDir {
	case 'N', 'n':
	case 'S', 's':
		lat = -lat
	default:
		return 0, 0, false
	}

	switch lonDir {
	case 'E', 'e':
	case 'W', 'w':
		lon = -lon
	default:
		return 0, 0, false
	}

	return lat, lon, true
}

// Check whether the packet should be retransmitted by the digipeater and if so, modifies the path.
func (p *Packet) CheckForRetransmit(stationCallsign string) (bool, error) {
	alreadyProcessed := false

	for i, component := range p.Path {
		if strings.HasPrefix(component, stationCallsign) {
			if strings.Contains(component, "*") {
				// already processed
				return false, nil
			}

			p.Path[i] = fmt.Sprintf("%s*", stationCallsign)
			alreadyProcessed = true
		}

		if strings.HasPrefix(component, "WIDE") && strings.Contains(component, "-") {
			parts := strings.Split(component, "-")

			if len(parts) == 2 {
				hopCount, err := strconv.Atoi(parts[1])
				if err != nil {
					return false, err
				}

				if hopCount <= 0 {
					// The packet has no more hops to give
					return false, nil
				}

				// It still has a hop to give so we need to decrement the hop count
				p.Path[i] = fmt.Sprintf("%s-%d", parts[0], hopCount-1)
			}
		}
	}

	if !alreadyProcessed {
		p.Path = append([]string{fmt.Sprintf("%s*", stationCallsign)}, p.Path...)
	}

	return true, nil
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
		Telemetry, WeatherReport, ObjectReport, ItemReport:
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

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

// Position extracts coordinates from standard uncompressed or compressed
// position reports, with or without the seven-byte APRS timestamp.
// Unsupported formats return ok=false; callers determine the fallback policy.
func (p *Packet) Position() (lat float64, lon float64, ok bool) {
	if len(p.Payload) == 0 {
		return 0, 0, false
	}
	offset := 1
	switch p.Payload[0] {
	case '!', '=':
	case '/', '@':
		// APRS timestamp: six ASCII digits followed by z, /, or h.
		if len(p.Payload) < 8 {
			return 0, 0, false
		}
		for k := 1; k <= 6; k++ {
			if !asciiDigit(p.Payload[k]) {
				return 0, 0, false
			}
		}
		first, _ := strconv.Atoi(p.Payload[1:3])
		second, _ := strconv.Atoi(p.Payload[3:5])
		third, _ := strconv.Atoi(p.Payload[5:7])
		switch p.Payload[7] {
		case 'z', '/':
			// Day-of-month, hour, minute.
			if first < 1 || first > 31 || second > 23 || third > 59 {
				return 0, 0, false
			}
		case 'h':
			// Hour, minute, second.
			if first > 23 || second > 59 || third > 59 {
				return 0, 0, false
			}
		default:
			return 0, 0, false
		}
		offset = 8
	default:
		return 0, 0, false
	}
	if len(p.Payload) <= offset {
		return 0, 0, false
	}

	// APRS 1.0.1 chapter 20 explicitly disallows numeric symbol-table
	// identifiers in *compressed* reports (a-j represent digit overlays).
	// A leading digit therefore unambiguously selects uncompressed syntax,
	// including malformed or truncated uncompressed frames.
	if asciiDigit(p.Payload[offset]) {
		return uncompressedPosition(p.Payload, offset)
	}
	return compressedPosition(p.Payload, offset)
}

func asciiDigit(c byte) bool { return c >= '0' && c <= '9' }

// Fixed DDMM.mmN / DDDMM.mmE syntax: ParseFloat alone accepts NaN,
// negatives and exponent notation, which are not valid APRS coordinates.
func uncompressedPosition(payload string, offset int) (lat float64, lon float64, ok bool) {
	if len(payload) < offset+19 {
		return 0, 0, false
	}
	latitude := payload[offset : offset+8]
	longitude := payload[offset+9 : offset+18]
	for _, k := range []int{0, 1, 2, 3, 5, 6} {
		if !asciiDigit(latitude[k]) {
			return 0, 0, false
		}
	}
	if latitude[4] != '.' {
		return 0, 0, false
	}
	for _, k := range []int{0, 1, 2, 3, 4, 6, 7} {
		if !asciiDigit(longitude[k]) {
			return 0, 0, false
		}
	}
	if longitude[5] != '.' {
		return 0, 0, false
	}
	// Both table ID and symbol code are mandatory, printable APRS bytes.
	if payload[offset+18] < '!' || payload[offset+18] > '~' {
		return 0, 0, false
	}
	// Only recognized symbol-table selectors count as valid positions.
	switch payload[offset+8] {
	case '/', '\\':
	default:
		if !asciiDigit(payload[offset+8]) &&
			(payload[offset+8] < 'A' || payload[offset+8] > 'Z') {
			return 0, 0, false
		}
	}
	latDegrees, _ := strconv.Atoi(latitude[:2])
	latMinutes, _ := strconv.ParseFloat(latitude[2:7], 64)
	lonDegrees, _ := strconv.Atoi(longitude[:3])
	lonMinutes, _ := strconv.ParseFloat(longitude[3:8], 64)
	if latDegrees > 90 || lonDegrees > 180 || latMinutes >= 60 || lonMinutes >= 60 {
		return 0, 0, false
	}
	lat = float64(latDegrees) + latMinutes/60
	lon = float64(lonDegrees) + lonMinutes/60
	switch latitude[7] {
	case 'N', 'n':
	case 'S', 's':
		lat = -lat
	default:
		return 0, 0, false
	}
	switch longitude[8] {
	case 'E', 'e':
	case 'W', 'w':
		lon = -lon
	default:
		return 0, 0, false
	}
	if lat > 90 || lat < -90 || lon > 180 || lon < -180 {
		return 0, 0, false
	}
	return lat, lon, true
}

// Compressed coordinates are four base-91 digits each, following the symbol
// table ID. APRS specifies /, backslash, A-Z and a-j for compressed tables;
// numeric overlays must use a-j, never a literal digit.
func compressedPosition(payload string, offset int) (lat float64, lon float64, ok bool) {
	if len(payload) < offset+13 {
		return 0, 0, false
	}
	table := payload[offset]
	switch {
	case table == '/' || table == '\\':
	case table >= 'A' && table <= 'Z':
	case table >= 'a' && table <= 'j':
	default:
		return 0, 0, false
	}
	if payload[offset+9] < '!' || payload[offset+9] > '~' {
		return 0, 0, false
	}
	decode := func(chars string) (int, bool) {
		value := 0
		for k := 0; k < len(chars); k++ {
			if chars[k] < '!' || chars[k] > '{' {
				return 0, false
			}
			value = value*91 + int(chars[k]-'!')
		}
		return value, true
	}
	y, ok := decode(payload[offset+1 : offset+5])
	if !ok {
		return 0, 0, false
	}
	x, ok := decode(payload[offset+5 : offset+9])
	if !ok {
		return 0, 0, false
	}
	lat = 90 - float64(y)/380926
	lon = -180 + float64(x)/190463
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
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

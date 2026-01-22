package digipeater

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/oorrwullie/go-igate/internal/aprs"
	"github.com/oorrwullie/go-igate/internal/config"
)

// Rewriter applies digipeater path rules without transmitting.
type Rewriter struct {
	callsign      string
	callsignUpper string
	aliasRules    []*regexp.Regexp
	wideRules     []*regexp.Regexp
	ssnPrefixes   map[string]struct{}
}

func NewRewriter(callsign string, cfg config.Digipeater) (*Rewriter, error) {
	if callsign == "" {
		return nil, fmt.Errorf("station callsign not configured")
	}

	alias, err := compilePatterns(cfg.AliasPatterns)
	if err != nil {
		return nil, fmt.Errorf("invalid alias pattern: %w", err)
	}

	wide, err := compilePatterns(cfg.WidePatterns)
	if err != nil {
		return nil, fmt.Errorf("invalid wide pattern: %w", err)
	}

	ssnPrefixes := make(map[string]struct{}, len(cfg.SSnPrefixes))
	for _, prefix := range cfg.SSnPrefixes {
		if prefix == "" {
			continue
		}
		ssnPrefixes[strings.ToUpper(prefix)] = struct{}{}
	}

	return &Rewriter{
		callsign:      callsign,
		callsignUpper: strings.ToUpper(callsign),
		aliasRules:    alias,
		wideRules:     wide,
		ssnPrefixes:   ssnPrefixes,
	}, nil
}

// Rewrite updates the packet path to reflect a local digipeat.
// Returns true when the path is rewritten.
func (r *Rewriter) Rewrite(packet *aprs.Packet) bool {
	if packet == nil {
		return false
	}

	if strings.EqualFold(packet.Src, r.callsign) {
		return false
	}

	if len(packet.Path) == 0 {
		return false
	}

	firstIdx := firstUnusedPathIndex(packet.Path)
	if firstIdx == -1 {
		return false
	}

	return r.rewritePath(packet, firstIdx)
}

func (r *Rewriter) rewritePath(packet *aprs.Packet, idx int) bool {
	component := strings.TrimSpace(packet.Path[idx])
	if component == "" {
		return false
	}

	bare := strings.TrimSuffix(component, "*")

	if strings.EqualFold(bare, r.callsign) {
		packet.Path[idx] = r.callsignUpper + "*"
		return true
	}

	for _, re := range r.aliasRules {
		if re.MatchString(bare) {
			packet.Path[idx] = r.callsignUpper + "*"
			return true
		}
	}

	for _, re := range r.wideRules {
		if re.MatchString(bare) {
			base, remaining, ok := parseHop(bare)
			if !ok || remaining <= 0 {
				return false
			}
			base = strings.ToUpper(base)

			if prefix, ok := ssnPrefix(base); ok && !r.allowSSnPrefix(prefix) {
				return false
			}

			if remaining == 1 {
				packet.Path[idx] = r.callsignUpper + "*"
				return true
			}

			packet.Path[idx] = fmt.Sprintf("%s-%d", base, remaining-1)
			packet.Path = insertBefore(packet.Path, idx, r.callsignUpper+"*")
			return true
		}
	}

	return false
}

func (r *Rewriter) allowSSnPrefix(prefix string) bool {
	if len(r.ssnPrefixes) == 0 {
		return false
	}

	_, ok := r.ssnPrefixes[strings.ToUpper(prefix)]
	return ok
}

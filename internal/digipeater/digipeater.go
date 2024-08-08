package digipeater

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/oorrwullie/go-igate/internal/log"
	"github.com/oorrwullie/go-igate/internal/pubsub"
	"github.com/oorrwullie/go-igate/internal/transmitter"
)

type Digipeater struct {
	tx        *transmitter.Tx
	InputChan <-chan string
	callsign  string
	logger    *log.Logger
	stop      chan bool
}

const minPacketSize = 35

func New(tx *transmitter.Tx, ps *pubsub.PubSub, callsign string, logger *log.Logger) *Digipeater {
	return &Digipeater{
		tx:        tx,
		InputChan: ps.Subscribe(),
		callsign:  callsign,
		logger:    logger,
		stop:      make(chan bool),
	}
}

func (d *Digipeater) Run() error {
	d.logger.Info("Starting digipeater...")

	for {
		select {
		case <-d.stop:
			return nil
		case msg := <-d.InputChan:
			d.HandleMessage(msg)
		}
	}
}

func (d *Digipeater) Stop() {
	d.logger.Info("Stopping digipeater...")
	d.stop <- true
}

func (d *Digipeater) HandleMessage(msg string) {

	if len(msg) < minPacketSize {
		d.logger.Error("Packet too short: ", msg)
		return
	}

	txMsg, err := d.fmtForTx(msg)
	if err != nil {
		d.logger.Error("Failed to parse message: ", err)
		return
	}

	d.tx.Send(txMsg)
}

func (d *Digipeater) fmtForTx(msg string) (string, error) {
	parts := strings.Split(msg, ">")
	if len(parts) != 2 {
		return "", fmt.Errorf("Invalid packet format")
	}

	source := parts[0]
	rest := parts[1]

	pathAndData := strings.SplitN(rest, ":", 2)
	if len(pathAndData) != 2 {
		return "", fmt.Errorf("Invalid packet format")
	}

	path := pathAndData[0]
	data := pathAndData[1]

	pathComponents := strings.Split(path, ",")

	updated := false
	for i, component := range pathComponents {
		if strings.HasPrefix(component, "WIDE") && !strings.Contains(component, "*") {
			wideParts := strings.Split(component, "-")
			if len(wideParts) == 2 {
				count, err := strconv.Atoi(wideParts[1])
				if err != nil || count <= 0 {
					continue
				}

				count--

				if count == 0 {
					pathComponents[i] = d.callsign + "*"
				} else {
					pathComponents[i] = d.callsign + "*," + wideParts[0] + "-" + strconv.Itoa(count)
				}

				updated = true
				break
			}
		}
	}

	if !updated {
		return "", fmt.Errorf("Packet has already been fully retransmitted or no valid WIDE component found")
	}

	newPath := strings.Join(pathComponents, ",")

	newPacket := fmt.Sprintf("%s>%s:%s", source, newPath, data)
	return newPacket, nil
}

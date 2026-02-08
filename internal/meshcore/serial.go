package meshcore

import (
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/watsoncj/meshcore-stats/internal/metrics"
	"go.bug.st/serial"
)

const (
	frameHeaderTx = '<' // client -> device
	frameHeaderRx = '>' // device -> client
	maxFrameSize  = 512
)

type Radio struct {
	port        serial.Port
	mu          sync.Mutex
	portName    string
	baudRate    int
	nodeName    string
	contactsMap map[string]string // pubkey prefix (4 hex chars) -> name
	pathByteMap map[byte]string   // path byte (1-byte hash) -> name
}

func Open(portName string, baudRate int) (*Radio, error) {
	r := &Radio{portName: portName, baudRate: baudRate}
	if err := r.openPort(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Radio) openPort() error {
	mode := &serial.Mode{
		BaudRate: r.baudRate,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	}

	port, err := serial.Open(r.portName, mode)
	if err != nil {
		return fmt.Errorf("failed to open serial port: %w", err)
	}

	if err := port.SetReadTimeout(2 * time.Second); err != nil {
		port.Close()
		return fmt.Errorf("failed to set read timeout: %w", err)
	}

	r.port = port
	return nil
}

func (r *Radio) Reconnect() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.port != nil {
		r.port.Close()
	}
	return r.openPort()
}

func (r *Radio) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.port.Close()
}

func (r *Radio) DrainPort() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.port.SetReadTimeout(100 * time.Millisecond)
	buf := make([]byte, maxFrameSize)
	for {
		n, _ := r.port.Read(buf)
		if n == 0 {
			break
		}
	}
	r.port.SetReadTimeout(2 * time.Second)
}

func (r *Radio) sendCommand(cmd []byte, expectedSize int) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	frame := make([]byte, 3+len(cmd))
	frame[0] = frameHeaderTx
	binary.LittleEndian.PutUint16(frame[1:3], uint16(len(cmd)))
	copy(frame[3:], cmd)

	if _, err := r.port.Write(frame); err != nil {
		return nil, fmt.Errorf("failed to write command: %w", err)
	}

	return r.readCommandResponse()
}

func (r *Radio) readCommandResponse() ([]byte, error) {
	for {
		data, err := r.readFrame()
		if err != nil {
			return nil, err
		}
		if len(data) > 0 && isPushCode(data[0]) {
			r.handlePushMessage(data)
			continue
		}
		return data, nil
	}
}

func (r *Radio) SetNodeName(name string) {
	r.nodeName = name
}

func (r *Radio) SetContacts(contacts []Contact) {
	r.contactsMap = make(map[string]string)
	r.pathByteMap = make(map[byte]string)
	for _, c := range contacts {
		prefix := fmt.Sprintf("%02X%02X", c.PubKey[0], c.PubKey[1])
		r.contactsMap[prefix] = c.Name
		// The path hash is just pub_key[0] (first byte of pubkey)
		// Note: collisions are possible but we just take the first match
		if _, exists := r.pathByteMap[c.PubKey[0]]; !exists {
			r.pathByteMap[c.PubKey[0]] = c.Name
		}
	}
}

func (r *Radio) AddSelfToContacts(info *SelfInfo) {
	if r.contactsMap == nil {
		r.contactsMap = make(map[string]string)
	}
	if r.pathByteMap == nil {
		r.pathByteMap = make(map[byte]string)
	}
	prefix := fmt.Sprintf("%02X%02X", info.PubKey[0], info.PubKey[1])
	r.contactsMap[prefix] = info.Name
	if _, exists := r.pathByteMap[info.PubKey[0]]; !exists {
		r.pathByteMap[info.PubKey[0]] = info.Name
	}
}

func (r *Radio) LookupSender(prefix string) string {
	if r.contactsMap == nil {
		return prefix
	}
	if name, ok := r.contactsMap[prefix]; ok {
		return name
	}
	return prefix
}

// LookupSenderByPathByte maps a 1-byte path hash to a contact name.
// MeshCore uses a single-byte truncated hash of the pubkey for path routing.
func (r *Radio) LookupSenderByPathByte(pathByte byte) string {
	if r.pathByteMap == nil {
		return fmt.Sprintf("%02X", pathByte)
	}
	if name, ok := r.pathByteMap[pathByte]; ok {
		return name
	}
	return fmt.Sprintf("%02X", pathByte)
}

func (r *Radio) handlePushMessage(data []byte) {
	if len(data) == 0 {
		return
	}
	switch data[0] {
	case PushCodeLogRxData:
		// Format: [0]=0x88, [1]=snr*4, [2]=rssi, [3+]=raw_packet
		// Raw packet: [0]=header, [1]=path_len, [2..]=path, remainder=encrypted_payload
		// The sender identity is encrypted and not directly extractable.
		// We can only track packets by "origin" = first hop in the path (the node we received from).
		if len(data) < 6 {
			return
		}
		snr := float64(int8(data[1])) / 4.0
		rssi := int8(data[2])
		rawPacket := data[3:]

		// Raw packet structure
		if len(rawPacket) < 3 {
			return
		}
		// header := rawPacket[0]
		pathLen := int(rawPacket[1])

		// The origin is the first hop in the path - this is the node we received from directly.
		// For zero-hop packets, the path is empty and we can't identify the sender.
		var origin string
		if pathLen > 0 && len(rawPacket) >= 2+pathLen {
			// First path byte is the immediate sender (1-byte truncated hash of pubkey)
			origin = r.LookupSenderByPathByte(rawPacket[2])
		} else {
			origin = "direct"
		}
		payloadLen := len(rawPacket) - 2 - pathLen

		node := r.nodeName
		if node == "" {
			node = "unknown"
		}
		metrics.MeshPacketsObserved.WithLabelValues(node, origin).Inc()
		metrics.MeshPacketRSSI.WithLabelValues(node, origin).Set(float64(rssi))
		metrics.MeshPacketSNR.WithLabelValues(node, origin).Set(snr)
		if payloadLen > 0 {
			metrics.MeshPacketBytes.WithLabelValues(node, origin).Add(float64(payloadLen))
		}
	}
}

func isPushCode(code byte) bool {
	return code >= 0x80
}

func (r *Radio) readFrame() ([]byte, error) {
	hdr := make([]byte, 3)
	if _, err := r.port.Read(hdr); err != nil {
		return nil, fmt.Errorf("failed to read frame header: %w", err)
	}

	if hdr[0] != frameHeaderRx {
		return nil, fmt.Errorf("invalid frame header: got 0x%02X, expected 0x%02X", hdr[0], frameHeaderRx)
	}

	frameLen := binary.LittleEndian.Uint16(hdr[1:3])
	if frameLen > maxFrameSize {
		return nil, fmt.Errorf("frame too large: %d", frameLen)
	}

	payload := make([]byte, frameLen)
	totalRead := 0
	for totalRead < int(frameLen) {
		n, err := r.port.Read(payload[totalRead:])
		if err != nil {
			return nil, fmt.Errorf("failed to read frame payload: %w", err)
		}
		totalRead += n
	}

	return payload, nil
}

func (r *Radio) GetVersion() (string, error) {
	data, err := r.sendCommand(BuildGetVersionCmd(), 0)
	if err != nil {
		return "", err
	}
	return ParseVersion(data)
}

func (r *Radio) GetStatsCore() (*StatsCore, error) {
	data, err := r.sendCommand(BuildGetStatsCmd(StatsTypeCore), StatsCoreSize)
	if err != nil {
		return nil, err
	}
	return ParseStatsCore(data)
}

func (r *Radio) GetStatsRadio() (*StatsRadio, error) {
	data, err := r.sendCommand(BuildGetStatsCmd(StatsTypeRadio), StatsRadioSize)
	if err != nil {
		return nil, err
	}
	return ParseStatsRadio(data)
}

func (r *Radio) GetStatsPackets() (*StatsPackets, error) {
	data, err := r.sendCommand(BuildGetStatsCmd(StatsTypePackets), StatsPacketsSize)
	if err != nil {
		return nil, err
	}
	return ParseStatsPackets(data)
}

func (r *Radio) AppStart() (*SelfInfo, error) {
	data, err := r.sendCommand(BuildAppStartCmd(), 0)
	if err != nil {
		return nil, err
	}
	return ParseSelfInfo(data)
}

func (r *Radio) GetContacts() ([]Contact, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	frame := make([]byte, 3+1)
	frame[0] = frameHeaderTx
	binary.LittleEndian.PutUint16(frame[1:3], 1)
	frame[3] = CmdGetContacts

	if _, err := r.port.Write(frame); err != nil {
		return nil, fmt.Errorf("failed to write command: %w", err)
	}

	// Read frames, skipping any push messages
	readResponseFrame := func() ([]byte, error) {
		for {
			data, err := r.readFrame()
			if err != nil {
				return nil, err
			}
			if len(data) > 0 && isPushCode(data[0]) {
				r.handlePushMessage(data)
				continue
			}
			return data, nil
		}
	}

	data, err := readResponseFrame()
	if err != nil {
		return nil, err
	}
	count, err := ParseContactsStart(data)
	if err != nil {
		return nil, err
	}

	contacts := make([]Contact, 0, count)
	for {
		data, err := readResponseFrame()
		if err != nil {
			return nil, err
		}
		if len(data) > 0 && data[0] == RespCodeEndOfContacts {
			break
		}
		contact, err := ParseContact(data)
		if err != nil {
			return nil, err
		}
		contacts = append(contacts, *contact)
	}
	return contacts, nil
}

func (r *Radio) SendLogin(pubKey []byte, password string) (uint32, error) {
	data, err := r.sendCommand(BuildSendLoginCmd(pubKey, password), 0)
	if err != nil {
		return 0, err
	}
	_, tag, _, err := ParseSentResponse(data)
	return tag, err
}

func (r *Radio) SendStatusReq(pubKey []byte) (uint32, error) {
	data, err := r.sendCommand(BuildSendStatusReqCmd(pubKey), 0)
	if err != nil {
		return 0, err
	}
	_, tag, _, err := ParseSentResponse(data)
	return tag, err
}

func (r *Radio) SendOwnerInfoReq(pubKey []byte) (uint32, error) {
	data, err := r.sendCommand(BuildSendOwnerInfoReqCmd(pubKey), 0)
	if err != nil {
		return 0, err
	}
	_, tag, _, err := ParseSentResponse(data)
	return tag, err
}

func (r *Radio) SendTelemetryReq(pubKey []byte) (uint32, error) {
	data, err := r.sendCommand(BuildSendTelemetryReqCmd(pubKey), 0)
	if err != nil {
		return 0, err
	}
	_, tag, _, err := ParseSentResponse(data)
	return tag, err
}

func (r *Radio) WaitForPush(timeout time.Duration) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.port.SetReadTimeout(timeout); err != nil {
		return nil, err
	}
	defer r.port.SetReadTimeout(2 * time.Second)

	return r.readFrame()
}

func (r *Radio) WaitForPushCode(wantCodes []byte, timeout time.Duration) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.port.SetReadTimeout(timeout); err != nil {
		return nil, err
	}
	defer r.port.SetReadTimeout(2 * time.Second)

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, err := r.readFrame()
		if err != nil {
			return nil, err
		}
		if len(data) == 0 {
			continue
		}
		for _, code := range wantCodes {
			if data[0] == code {
				return data, nil
			}
		}
	}
	return nil, fmt.Errorf("timeout waiting for response")
}

func (r *Radio) SetRadioParams(freqKHz uint32, bwHz uint32, sf uint8, cr uint8) error {
	data, err := r.sendCommand(BuildSetRadioParamsCmd(freqKHz, bwHz, sf, cr), 0)
	if err != nil {
		return err
	}
	if len(data) > 0 && data[0] == RespCodeOK {
		return nil
	}
	if len(data) > 0 && data[0] == RespCodeErr {
		return fmt.Errorf("radio rejected parameters (error code %d)", data[1])
	}
	return fmt.Errorf("unexpected response: 0x%02X", data[0])
}

func (r *Radio) SetRadioTxPower(powerDBm uint8) error {
	data, err := r.sendCommand(BuildSetRadioTxPowerCmd(powerDBm), 0)
	if err != nil {
		return err
	}
	if len(data) > 0 && data[0] == RespCodeOK {
		return nil
	}
	return fmt.Errorf("unexpected response: 0x%02X", data[0])
}

func (r *Radio) Reboot() error {
	data, err := r.sendCommand(BuildRebootCmd(), 0)
	if err != nil {
		return err
	}
	if len(data) > 0 && data[0] == RespCodeOK {
		return nil
	}
	return fmt.Errorf("unexpected response: 0x%02X", data[0])
}

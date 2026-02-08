package meshcore

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

const (
	CmdAppStart        = 1
	CmdGetContacts     = 4
	CmdGetVersion      = 10
	CmdSetRadioParams  = 11
	CmdSetRadioTxPower = 12
	CmdReboot          = 19
	CmdSendLogin       = 26
	CmdSendStatusReq   = 27
	CmdSendBinaryReq   = 50
	CmdGetStats        = 56

	ReqTypeGetOwnerInfo = 0x07

	StatsTypeCore    = 0
	StatsTypeRadio   = 1
	StatsTypePackets = 2

	RespCodeOK            = 0
	RespCodeErr           = 1
	RespCodeContactsStart = 2
	RespCodeContact       = 3
	RespCodeEndOfContacts = 4
	RespCodeSelfInfo      = 5
	RespCodeSent          = 6
	RespCodeVersion       = 8
	RespCodeStats         = 24

	PushCodeLoginSuccess    = 0x85
	PushCodeLoginFail       = 0x86
	PushCodeStatusResponse  = 0x87
	PushCodeLogRxData       = 0x88
	PushCodeBinaryResponse  = 0x8C

	PubKeySize       = 32
	StatsCoreSize    = 11
	StatsRadioSize   = 14
	StatsPacketsSize = 26
)

type Contact struct {
	PubKey     [PubKeySize]byte
	Type       uint8
	Flags      uint8
	Name       string
	OutPathLen int8
	Lat        float64
	Lon        float64
}

type SelfInfo struct {
	PubKey  [PubKeySize]byte
	Name    string
	Lat     float64
	Lon     float64
	TxPower uint8
	MaxTx   uint8
}

type StatsCore struct {
	BatteryMV  uint16
	UptimeSecs uint32
	Errors     uint16
	QueueLen   uint8
}

type StatsRadio struct {
	NoiseFloor int16
	LastRSSI   int8
	LastSNR    float64 // scaled by 4 in protocol
	TxAirSecs  uint32
	RxAirSecs  uint32
}

type StatsPackets struct {
	Recv     uint32
	Sent     uint32
	FloodTx  uint32
	DirectTx uint32
	FloodRx  uint32
	DirectRx uint32
}

func BuildGetStatsCmd(statsType uint8) []byte {
	return []byte{CmdGetStats, statsType}
}

func BuildGetVersionCmd() []byte {
	return []byte{CmdGetVersion}
}

func BuildAppStartCmd() []byte {
	cmd := make([]byte, 11)
	cmd[0] = CmdAppStart
	cmd[1] = 0x03
	copy(cmd[2:], []byte("mccli"))
	return cmd
}

func BuildGetContactsCmd() []byte {
	return []byte{CmdGetContacts}
}

func BuildSendLoginCmd(pubKey []byte, password string) []byte {
	cmd := make([]byte, 1+PubKeySize+len(password))
	cmd[0] = CmdSendLogin
	copy(cmd[1:1+PubKeySize], pubKey)
	copy(cmd[1+PubKeySize:], password)
	return cmd
}

func BuildSendStatusReqCmd(pubKey []byte) []byte {
	cmd := make([]byte, 1+PubKeySize)
	cmd[0] = CmdSendStatusReq
	copy(cmd[1:], pubKey)
	return cmd
}

func BuildSendOwnerInfoReqCmd(pubKey []byte) []byte {
	cmd := make([]byte, 1+PubKeySize+4+1)
	cmd[0] = CmdSendBinaryReq
	copy(cmd[1:1+PubKeySize], pubKey)
	// 4 reserved bytes (zeros)
	cmd[1+PubKeySize+4] = ReqTypeGetOwnerInfo
	return cmd
}

func BuildSetRadioParamsCmd(freqKHz uint32, bwHz uint32, sf uint8, cr uint8) []byte {
	cmd := make([]byte, 11)
	cmd[0] = CmdSetRadioParams
	binary.LittleEndian.PutUint32(cmd[1:5], freqKHz)
	binary.LittleEndian.PutUint32(cmd[5:9], bwHz)
	cmd[9] = sf
	cmd[10] = cr
	return cmd
}

func BuildSetRadioTxPowerCmd(powerDBm uint8) []byte {
	return []byte{CmdSetRadioTxPower, powerDBm}
}

func BuildRebootCmd() []byte {
	return []byte{CmdReboot}
}

type RadioRegion struct {
	Name    string
	FreqKHz uint32
	BwHz    uint32
	SF      uint8
	CR      uint8
}

var Regions = map[string]RadioRegion{
	"US": {Name: "US", FreqKHz: 910525, BwHz: 62500, SF: 7, CR: 5},
	"EU": {Name: "EU", FreqKHz: 869525, BwHz: 250000, SF: 10, CR: 5},
	"AU": {Name: "AU", FreqKHz: 915000, BwHz: 250000, SF: 10, CR: 5},
	"NZ": {Name: "NZ", FreqKHz: 915000, BwHz: 250000, SF: 10, CR: 5},
}

func ParseSelfInfo(data []byte) (*SelfInfo, error) {
	// Format: [0]=code, [1]=adv_type, [2]=tx_power, [3]=max_tx_power,
	// [4-35]=pub_key(32), [36-39]=lat, [40-43]=lon, [44-47]=flags(4),
	// [48-51]=freq, [52-55]=bw, [56]=sf, [57]=cr, [58+]=name
	const headerSize = 58
	if len(data) < headerSize {
		return nil, fmt.Errorf("insufficient data for self info: got %d bytes, data=%X", len(data), data)
	}
	if data[0] != RespCodeSelfInfo {
		return nil, fmt.Errorf("unexpected response code: 0x%02X", data[0])
	}
	info := &SelfInfo{}
	copy(info.PubKey[:], data[4:4+PubKeySize])
	info.Lat = float64(int32(binary.LittleEndian.Uint32(data[36:40]))) / 1e6
	info.Lon = float64(int32(binary.LittleEndian.Uint32(data[40:44]))) / 1e6
	if len(data) > headerSize {
		info.Name = trimNull(data[headerSize:])
	}
	return info, nil
}

func trimNull(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

func ParseVersion(data []byte) (string, error) {
	if len(data) < 1 {
		return "", fmt.Errorf("empty response")
	}
	if data[0] != RespCodeVersion {
		return "", fmt.Errorf("unexpected response code: 0x%02X", data[0])
	}
	if len(data) == 1 {
		return "unknown", nil
	}
	return trimNull(data[1:]), nil
}

func ParseOwnerInfoResponse(data []byte) (version, nodeName, ownerInfo string, err error) {
	// Format: [0]=code, [1-6]=sender prefix, [7]=reserved, [8-11]=timestamp, [12+]=payload
	// Payload format: "version\nnode_name\nowner_info"
	if len(data) < 13 {
		return "", "", "", fmt.Errorf("insufficient data for owner info: %d", len(data))
	}
	if data[0] != PushCodeBinaryResponse {
		return "", "", "", fmt.Errorf("unexpected response code: 0x%02X", data[0])
	}
	payload := trimNull(data[12:])
	parts := strings.SplitN(payload, "\n", 3)
	if len(parts) >= 1 {
		version = parts[0]
	}
	if len(parts) >= 2 {
		nodeName = parts[1]
	}
	if len(parts) >= 3 {
		ownerInfo = parts[2]
	}
	return version, nodeName, ownerInfo, nil
}

func ParseContactsStart(data []byte) (uint32, error) {
	if len(data) < 5 {
		return 0, fmt.Errorf("insufficient data for contacts start: %d", len(data))
	}
	if data[0] != RespCodeContactsStart {
		return 0, fmt.Errorf("unexpected response code: 0x%02X", data[0])
	}
	return binary.LittleEndian.Uint32(data[1:5]), nil
}

func ParseContact(data []byte) (*Contact, error) {
	// Format: [0]=code, [1-32]=pub_key(32), [33]=type, [34]=flags,
	// [35]=out_path_len, [36-99]=out_path(64), [100-131]=name(32),
	// [132-135]=last_advert_ts, [136-139]=lat, [140-143]=lon, [144-147]=lastmod
	const (
		maxPathSize = 64
		nameOffset  = 1 + PubKeySize + 3 + maxPathSize // 1+32+3+64 = 100
		nameSize    = 32
		minSize     = 148 // need lat/lon
	)
	if len(data) < minSize {
		return nil, fmt.Errorf("insufficient data for contact: %d", len(data))
	}
	if data[0] != RespCodeContact {
		return nil, fmt.Errorf("unexpected response code: 0x%02X", data[0])
	}
	c := &Contact{}
	copy(c.PubKey[:], data[1:1+PubKeySize])
	c.Type = data[1+PubKeySize]
	c.Flags = data[1+PubKeySize+1]
	c.OutPathLen = int8(data[1+PubKeySize+2])
	c.Name = trimNull(data[nameOffset : nameOffset+nameSize])
	c.Lat = float64(int32(binary.LittleEndian.Uint32(data[136:140]))) / 1e6
	c.Lon = float64(int32(binary.LittleEndian.Uint32(data[140:144]))) / 1e6
	return c, nil
}

func ParseSentResponse(data []byte) (isFlood bool, tag uint32, timeout uint32, err error) {
	if len(data) < 10 {
		return false, 0, 0, fmt.Errorf("insufficient data for sent response: %d", len(data))
	}
	if data[0] != RespCodeSent {
		return false, 0, 0, fmt.Errorf("unexpected response code: 0x%02X", data[0])
	}
	isFlood = data[1] == 1
	tag = binary.LittleEndian.Uint32(data[2:6])
	timeout = binary.LittleEndian.Uint32(data[6:10])
	return isFlood, tag, timeout, nil
}

func ParseLoginSuccess(data []byte) (pubKeyPrefix []byte, err error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("insufficient data for login success: %d", len(data))
	}
	if data[0] != PushCodeLoginSuccess {
		return nil, fmt.Errorf("unexpected response code: 0x%02X", data[0])
	}
	return data[2:8], nil
}

func ParseStatusResponse(data []byte) (*StatsCore, *StatsRadio, *StatsPackets, error) {
	if len(data) < 8 {
		return nil, nil, nil, fmt.Errorf("insufficient data for status response: %d", len(data))
	}
	if data[0] != PushCodeStatusResponse {
		return nil, nil, nil, fmt.Errorf("unexpected response code: 0x%02X", data[0])
	}

	if len(data) < 48 {
		return nil, nil, nil, fmt.Errorf("insufficient status data: %d", len(data))
	}

	core := &StatsCore{
		BatteryMV:  binary.LittleEndian.Uint16(data[8:10]),
		QueueLen:   data[10],
		UptimeSecs: binary.LittleEndian.Uint32(data[28:32]),
	}

	radio := &StatsRadio{
		LastRSSI:  int8(data[12]),
		LastSNR:   float64(int8(data[14])) / 4.0,
		TxAirSecs: binary.LittleEndian.Uint32(data[24:28]),
		RxAirSecs: binary.LittleEndian.Uint32(data[56:60]),
	}

	packets := &StatsPackets{
		Recv:     binary.LittleEndian.Uint32(data[16:20]),
		Sent:     binary.LittleEndian.Uint32(data[20:24]),
		FloodTx:  binary.LittleEndian.Uint32(data[32:36]),
		DirectTx: binary.LittleEndian.Uint32(data[36:40]),
		FloodRx:  binary.LittleEndian.Uint32(data[40:44]),
		DirectRx: binary.LittleEndian.Uint32(data[44:48]),
	}

	return core, radio, packets, nil
}

func ParseStatsCore(data []byte) (*StatsCore, error) {
	if len(data) < StatsCoreSize {
		return nil, fmt.Errorf("insufficient data: got %d, need %d", len(data), StatsCoreSize)
	}
	if data[0] != RespCodeStats || data[1] != StatsTypeCore {
		return nil, errors.New("invalid response type for core stats")
	}
	return &StatsCore{
		BatteryMV:  binary.LittleEndian.Uint16(data[2:4]),
		UptimeSecs: binary.LittleEndian.Uint32(data[4:8]),
		Errors:     binary.LittleEndian.Uint16(data[8:10]),
		QueueLen:   data[10],
	}, nil
}

func ParseStatsRadio(data []byte) (*StatsRadio, error) {
	if len(data) < StatsRadioSize {
		return nil, fmt.Errorf("insufficient data: got %d, need %d", len(data), StatsRadioSize)
	}
	if data[0] != RespCodeStats || data[1] != StatsTypeRadio {
		return nil, errors.New("invalid response type for radio stats")
	}
	return &StatsRadio{
		NoiseFloor: int16(binary.LittleEndian.Uint16(data[2:4])),
		LastRSSI:   int8(data[4]),
		LastSNR:    float64(int8(data[5])) / 4.0,
		TxAirSecs:  binary.LittleEndian.Uint32(data[6:10]),
		RxAirSecs:  binary.LittleEndian.Uint32(data[10:14]),
	}, nil
}

func ParseStatsPackets(data []byte) (*StatsPackets, error) {
	if len(data) < StatsPacketsSize {
		return nil, fmt.Errorf("insufficient data: got %d, need %d", len(data), StatsPacketsSize)
	}
	if data[0] != RespCodeStats || data[1] != StatsTypePackets {
		return nil, errors.New("invalid response type for packet stats")
	}
	return &StatsPackets{
		Recv:     binary.LittleEndian.Uint32(data[2:6]),
		Sent:     binary.LittleEndian.Uint32(data[6:10]),
		FloodTx:  binary.LittleEndian.Uint32(data[10:14]),
		DirectTx: binary.LittleEndian.Uint32(data[14:18]),
		FloodRx:  binary.LittleEndian.Uint32(data[18:22]),
		DirectRx: binary.LittleEndian.Uint32(data[22:26]),
	}, nil
}

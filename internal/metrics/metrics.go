package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	BatteryMillivolts = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "meshcore_battery_millivolts",
		Help: "Battery voltage in millivolts",
	}, []string{"node"})

	TemperatureCelsius = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "meshcore_temperature_celsius",
		Help: "Device temperature in degrees Celsius",
	}, []string{"node"})

	UptimeSeconds = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "meshcore_uptime_seconds",
		Help: "Device uptime in seconds",
	}, []string{"node"})

	ErrorFlags = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "meshcore_error_flags",
		Help: "Error flags bitmask",
	}, []string{"node"})

	QueueLength = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "meshcore_queue_length",
		Help: "Outbound packet queue length",
	}, []string{"node"})

	NoiseFloorDBm = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "meshcore_noise_floor_dbm",
		Help: "Radio noise floor in dBm",
	}, []string{"node"})

	LastRSSI = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "meshcore_last_rssi_dbm",
		Help: "Last received signal strength in dBm",
	}, []string{"node"})

	LastSNR = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "meshcore_last_snr_db",
		Help: "Last signal-to-noise ratio in dB",
	}, []string{"node"})

	TxAirtimeSeconds = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "meshcore_tx_airtime_seconds_total",
		Help: "Cumulative transmit airtime in seconds",
	}, []string{"node"})

	RxAirtimeSeconds = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "meshcore_rx_airtime_seconds_total",
		Help: "Cumulative receive airtime in seconds",
	}, []string{"node"})

	PacketsReceived = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "meshcore_packets_received_total",
		Help: "Total packets received",
	}, []string{"node"})

	PacketsSent = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "meshcore_packets_sent_total",
		Help: "Total packets sent",
	}, []string{"node"})

	PacketsFloodTx = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "meshcore_packets_flood_tx_total",
		Help: "Packets sent via flood routing",
	}, []string{"node"})

	PacketsDirectTx = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "meshcore_packets_direct_tx_total",
		Help: "Packets sent via direct routing",
	}, []string{"node"})

	PacketsFloodRx = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "meshcore_packets_flood_rx_total",
		Help: "Packets received via flood routing",
	}, []string{"node"})

	PacketsDirectRx = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "meshcore_packets_direct_rx_total",
		Help: "Packets received via direct routing",
	}, []string{"node"})

	ScrapeErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "meshcore_scrape_errors_total",
		Help: "Total number of scrape errors",
	}, []string{"node"})

	LoginStatus = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "meshcore_login_status",
		Help: "Login status (1=logged in, 0=not logged in)",
	}, []string{"node"})

	// Mesh traffic metrics (from push log data)
	MeshPacketsObserved = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "meshcore_mesh_packets_observed_total",
		Help: "Mesh packets observed by the repeater",
	}, []string{"node", "sender"})

	MeshPacketRSSI = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "meshcore_mesh_packet_rssi_dbm",
		Help: "Last RSSI of packets from a mesh sender",
	}, []string{"node", "sender"})

	MeshPacketSNR = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "meshcore_mesh_packet_snr_db",
		Help: "Last SNR of packets from a mesh sender",
	}, []string{"node", "sender"})

	MeshPacketBytes = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "meshcore_mesh_packet_bytes_total",
		Help: "Total bytes observed from mesh senders",
	}, []string{"node", "sender"})

	RepeaterLogins = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "meshcore_repeater_logins_total",
		Help: "Total successful repeater logins",
	}, []string{"node"})

	RadioReboots = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "meshcore_radio_reboots_total",
		Help: "Total companion radio reboot commands sent",
	}, []string{"node"})

	SerialReconnects = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "meshcore_serial_reconnects_total",
		Help: "Total serial port reconnections",
	}, []string{"node"})

	// Node position metrics
	NodeLatitude = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "meshcore_node_latitude",
		Help: "Node latitude in degrees",
	}, []string{"node"})

	NodeLongitude = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "meshcore_node_longitude",
		Help: "Node longitude in degrees",
	}, []string{"node"})


)

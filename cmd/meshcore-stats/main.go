package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/watsoncj/meshcore-stats/internal/meshcore"
	"github.com/watsoncj/meshcore-stats/internal/metrics"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "set-region" {
		setRegionCmd()
		return
	}

	port := flag.String("port", "/dev/ttyACM0", "Serial port for MeshCore radio")
	baud := flag.Int("baud", 115200, "Baud rate")
	addr := flag.String("addr", ":9200", "Address to expose metrics on")
	interval := flag.Duration("interval", 10*time.Minute, "Scrape interval")
	repeater := flag.String("repeater", "", "Repeater name to login and query stats from")
	password := flag.String("password", "", "Password for repeater login")
	flag.Parse()

	log.Printf("Opening serial port %s at %d baud", *port, *baud)
	radio, err := meshcore.Open(*port, *baud)
	if err != nil {
		log.Fatalf("Failed to open radio: %v", err)
	}
	defer radio.Close()

	if *repeater != "" {
		go collectRemoteMetrics(radio, *interval, *repeater, *password)
	} else {
		go collectLocalMetrics(radio, *interval)
	}

	log.Printf("Serving metrics on %s/metrics", *addr)
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(*addr, nil))
}

func setRegionCmd() {
	fs := flag.NewFlagSet("set-region", flag.ExitOnError)
	port := fs.String("port", "/dev/ttyACM0", "Serial port for MeshCore radio")
	baud := fs.Int("baud", 115200, "Baud rate")
	region := fs.String("region", "", "Region code (US, EU, AU, NZ)")
	txPower := fs.Int("tx-power", 0, "TX power in dBm (optional, 1-22)")
	fs.Parse(os.Args[2:])

	if *region == "" {
		fmt.Println("Available regions:")
		for code, r := range meshcore.Regions {
			fmt.Printf("  %s: %.3f MHz, %d kHz BW, SF%d, CR%d\n",
				code, float64(r.FreqKHz)/1000.0, r.BwHz/1000, r.SF, r.CR)
		}
		fmt.Println("\nUsage: meshcore-stats set-region -region US [-port /dev/ttyACM0]")
		os.Exit(1)
	}

	r, ok := meshcore.Regions[strings.ToUpper(*region)]
	if !ok {
		fmt.Printf("Unknown region: %s\n", *region)
		fmt.Println("Available: US, EU, AU, NZ")
		os.Exit(1)
	}

	log.Printf("Opening serial port %s at %d baud", *port, *baud)
	radio, err := meshcore.Open(*port, *baud)
	if err != nil {
		log.Fatalf("Failed to open radio: %v", err)
	}
	defer radio.Close()

	log.Printf("Setting region to %s (%.3f MHz, %d kHz BW, SF%d, CR%d)...",
		r.Name, float64(r.FreqKHz)/1000.0, r.BwHz/1000, r.SF, r.CR)

	if err := radio.SetRadioParams(r.FreqKHz, r.BwHz, r.SF, r.CR); err != nil {
		log.Fatalf("Failed to set radio params: %v", err)
	}
	log.Println("Radio parameters set successfully")

	if *txPower > 0 {
		log.Printf("Setting TX power to %d dBm...", *txPower)
		if err := radio.SetRadioTxPower(uint8(*txPower)); err != nil {
			log.Fatalf("Failed to set TX power: %v", err)
		}
		log.Println("TX power set successfully")
	}

	log.Println("Done! Radio is now configured for", r.Name)
}

func collectLocalMetrics(radio *meshcore.Radio, interval time.Duration) {
	const node = "local"
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	collect := func() {
		if core, err := radio.GetStatsCore(); err != nil {
			log.Printf("Error getting core stats: %v", err)
			metrics.ScrapeErrors.WithLabelValues(node).Inc()
		} else {
			metrics.BatteryMillivolts.WithLabelValues(node).Set(float64(core.BatteryMV))
			metrics.UptimeSeconds.WithLabelValues(node).Set(float64(core.UptimeSecs))
			metrics.ErrorFlags.WithLabelValues(node).Set(float64(core.Errors))
			metrics.QueueLength.WithLabelValues(node).Set(float64(core.QueueLen))
		}

		if radioStats, err := radio.GetStatsRadio(); err != nil {
			log.Printf("Error getting radio stats: %v", err)
			metrics.ScrapeErrors.WithLabelValues(node).Inc()
		} else {
			metrics.NoiseFloorDBm.WithLabelValues(node).Set(float64(radioStats.NoiseFloor))
			metrics.LastRSSI.WithLabelValues(node).Set(float64(radioStats.LastRSSI))
			metrics.LastSNR.WithLabelValues(node).Set(radioStats.LastSNR)
			metrics.TxAirtimeSeconds.WithLabelValues(node).Set(float64(radioStats.TxAirSecs))
			metrics.RxAirtimeSeconds.WithLabelValues(node).Set(float64(radioStats.RxAirSecs))
		}

		if packets, err := radio.GetStatsPackets(); err != nil {
			log.Printf("Error getting packet stats: %v", err)
			metrics.ScrapeErrors.WithLabelValues(node).Inc()
		} else {
			metrics.PacketsReceived.WithLabelValues(node).Set(float64(packets.Recv))
			metrics.PacketsSent.WithLabelValues(node).Set(float64(packets.Sent))
			metrics.PacketsFloodTx.WithLabelValues(node).Set(float64(packets.FloodTx))
			metrics.PacketsDirectTx.WithLabelValues(node).Set(float64(packets.DirectTx))
			metrics.PacketsFloodRx.WithLabelValues(node).Set(float64(packets.FloodRx))
			metrics.PacketsDirectRx.WithLabelValues(node).Set(float64(packets.DirectRx))
		}
	}

	collect()
	for range ticker.C {
		collect()
	}
}

func collectRemoteMetrics(radio *meshcore.Radio, interval time.Duration, repeaterName, password string) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var targetContact *meshcore.Contact
	var loggedIn bool

	collect := func() {
		if targetContact == nil {
			log.Printf("Initializing companion radio...")
			selfInfo, err := radio.AppStart()
			if err != nil {
				log.Printf("Error starting app: %v", err)
				metrics.ScrapeErrors.WithLabelValues(repeaterName).Inc()
				return
			}
			log.Printf("Connected as: %s (%.6f, %.6f)", selfInfo.Name, selfInfo.Lat, selfInfo.Lon)
			radio.AddSelfToContacts(selfInfo)
			if selfInfo.Lat != 0 || selfInfo.Lon != 0 {
				metrics.NodeLatitude.WithLabelValues(selfInfo.Name).Set(selfInfo.Lat)
				metrics.NodeLongitude.WithLabelValues(selfInfo.Name).Set(selfInfo.Lon)
			}

			log.Printf("Getting contacts...")
			contacts, err := radio.GetContacts()
			if err != nil {
				log.Printf("Error getting contacts: %v", err)
				metrics.ScrapeErrors.WithLabelValues(repeaterName).Inc()
				return
			}

			radio.SetContacts(contacts)
			for i := range contacts {
				c := &contacts[i]
				if c.Lat != 0 || c.Lon != 0 {
					metrics.NodeLatitude.WithLabelValues(c.Name).Set(c.Lat)
					metrics.NodeLongitude.WithLabelValues(c.Name).Set(c.Lon)
				}
				if strings.EqualFold(c.Name, repeaterName) {
					targetContact = c
					log.Printf("Found repeater: %s (type=%d) at (%.6f, %.6f)", c.Name, c.Type, c.Lat, c.Lon)
				}
			}

			if targetContact == nil {
				log.Printf("Repeater '%s' not found in contacts. Available:", repeaterName)
				for _, c := range contacts {
					log.Printf("  - %s (type=%d)", c.Name, c.Type)
				}
				return
			}
		}

		if !loggedIn {
			log.Printf("Logging into repeater %s...", targetContact.Name)
			radio.SetNodeName(repeaterName)
			_, err := radio.SendLogin(targetContact.PubKey[:], password)
			if err != nil {
				log.Printf("Error sending login: %v", err)
				metrics.ScrapeErrors.WithLabelValues(repeaterName).Inc()
				return
			}

			loginCodes := []byte{meshcore.PushCodeLoginSuccess, meshcore.PushCodeLoginFail}
			data, err := radio.WaitForPushCode(loginCodes, 30*time.Second)
			if err != nil {
				log.Printf("Error waiting for login response: %v", err)
				metrics.ScrapeErrors.WithLabelValues(repeaterName).Inc()
				return
			}

			if data[0] == meshcore.PushCodeLoginSuccess {
				log.Printf("Login successful!")
				loggedIn = true
				metrics.LoginStatus.WithLabelValues(repeaterName).Set(1)
			} else {
				log.Printf("Login failed!")
				metrics.LoginStatus.WithLabelValues(repeaterName).Set(0)
				return
			}
		}

		log.Printf("Requesting status from %s...", targetContact.Name)
		_, err := radio.SendStatusReq(targetContact.PubKey[:])
		if err != nil {
			log.Printf("Error sending status request: %v", err)
			metrics.ScrapeErrors.WithLabelValues(repeaterName).Inc()
			loggedIn = false
			return
		}

		statusCodes := []byte{meshcore.PushCodeStatusResponse}
		data, err := radio.WaitForPushCode(statusCodes, 30*time.Second)
		if err != nil {
			log.Printf("Error waiting for status response: %v", err)
			metrics.ScrapeErrors.WithLabelValues(repeaterName).Inc()
			loggedIn = false
			return
		}

		if data[0] == meshcore.PushCodeStatusResponse {
			core, radioStats, packets, err := meshcore.ParseStatusResponse(data)
			if err != nil {
				log.Printf("Error parsing status response: %v", err)
				metrics.ScrapeErrors.WithLabelValues(repeaterName).Inc()
				return
			}

			metrics.BatteryMillivolts.WithLabelValues(repeaterName).Set(float64(core.BatteryMV))
			metrics.UptimeSeconds.WithLabelValues(repeaterName).Set(float64(core.UptimeSecs))
			metrics.QueueLength.WithLabelValues(repeaterName).Set(float64(core.QueueLen))

			metrics.LastRSSI.WithLabelValues(repeaterName).Set(float64(radioStats.LastRSSI))
			metrics.LastSNR.WithLabelValues(repeaterName).Set(radioStats.LastSNR)
			metrics.TxAirtimeSeconds.WithLabelValues(repeaterName).Set(float64(radioStats.TxAirSecs))

			metrics.PacketsReceived.WithLabelValues(repeaterName).Set(float64(packets.Recv))
			metrics.PacketsSent.WithLabelValues(repeaterName).Set(float64(packets.Sent))
			metrics.PacketsFloodTx.WithLabelValues(repeaterName).Set(float64(packets.FloodTx))
			metrics.PacketsDirectTx.WithLabelValues(repeaterName).Set(float64(packets.DirectTx))
			metrics.PacketsFloodRx.WithLabelValues(repeaterName).Set(float64(packets.FloodRx))
			metrics.PacketsDirectRx.WithLabelValues(repeaterName).Set(float64(packets.DirectRx))

			log.Printf("Stats: battery=%dmV, rssi=%d, snr=%.1f, rx=%d (flood=%d, direct=%d), tx=%d (flood=%d, direct=%d)",
				core.BatteryMV, radioStats.LastRSSI, radioStats.LastSNR,
				packets.Recv, packets.FloodRx, packets.DirectRx,
				packets.Sent, packets.FloodTx, packets.DirectTx)
		} else {
			log.Printf("Unexpected response: 0x%02X", data[0])
		}
	}

	collect()
	for range ticker.C {
		collect()
	}
}

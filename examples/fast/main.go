package main

import (
	"fmt"
	"log"
	"time"

	"github.com/metalgrid/tr069-cel-mapper/pkg/extractor"
	"github.com/metalgrid/tr069-cel-mapper/pkg/mapper"
	"github.com/metalgrid/tr069-cel-mapper/pkg/registry"
	"github.com/metalgrid/tr069-cel-mapper/pkg/router"
)

type Host struct {
	MACAddress    string
	IPAddress     string
	HostName      string
	Active        bool
	InterfaceType string
}

type Wifi struct {
	SSID     string
	Password string
	Channel  int
	Band     string
	Enabled  bool
}

type WANPPPConnection struct {
	Enable           bool
	ConnectionStatus string
	ConnectionType   string
	Name             string
	Username         string
	ExternalIP       string
	DNSServers       string
	Uptime           int64
}

func main() {
	reg := registry.New()
	reg.MustRegister("host", func() any { return &Host{} })
	reg.MustRegister("wifi", func() any { return &Wifi{} })
	reg.MustRegister("wanppp", func() any { return &WANPPPConnection{} })

	fastMapper := mapper.NewFast(reg, mapper.WithFastStats())

	setupHostRules(fastMapper)
	setupWifiRules(fastMapper)
	setupWANRules(fastMapper)

	testData := [][2]string{
		{"InternetGatewayDevice.LANDevice.1.Hosts.1.MACAddress", "AA:BB:CC:DD:EE:FF"},
		{"InternetGatewayDevice.LANDevice.1.Hosts.1.IPAddress", "192.168.1.100"},
		{"InternetGatewayDevice.LANDevice.1.Hosts.1.HostName", "laptop-john"},
		{"InternetGatewayDevice.LANDevice.1.Hosts.1.Active", "true"},
		{"InternetGatewayDevice.LANDevice.1.Hosts.1.InterfaceType", "Ethernet"},

		{"Device.Hosts.Host.2.PhysAddress", "11-22-33-44-55-66"},
		{"Device.Hosts.Host.2.IPAddress", "192.168.1.101"},
		{"Device.Hosts.Host.2.HostName", "phone-mary"},
		{"Device.Hosts.Host.2.Active", "1"},

		{"InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.SSID", "HomeNetwork"},
		{"InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.KeyPassphrase", "SecretPass123"},
		{"InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.Channel", "6"},
		{"InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.Enable", "true"},

		{"Device.WiFi.AccessPoint.2.SSID", "GuestNetwork"},
		{"Device.WiFi.AccessPoint.2.Security.KeyPassphrase", "Guest2024"},
		{"Device.WiFi.Radio.2.Channel", "149"},
		{"Device.WiFi.AccessPoint.2.Enable", "false"},

		{"InternetGatewayDevice.WANDevice.1.WANConnectionDevice.1.WANPPPConnection.1.Enable", "true"},
		{"InternetGatewayDevice.WANDevice.1.WANConnectionDevice.1.WANPPPConnection.1.ConnectionStatus", "Connected"},
		{"InternetGatewayDevice.WANDevice.1.WANConnectionDevice.1.WANPPPConnection.1.ExternalIPAddress", "203.0.113.42"},
		{"InternetGatewayDevice.WANDevice.1.WANConnectionDevice.1.WANPPPConnection.1.Uptime", "86400"},
	}

	start := time.Now()

	for _, data := range testData {
		if err := fastMapper.Process(data[0], data[1]); err != nil {
			log.Printf("Error processing %s: %v", data[0], err)
		}
	}

	elapsed := time.Since(start)

	fmt.Println("=== Results ===")
	store := fastMapper.GetStore()
	store.ForEach(func(target, key string, obj any) error {
		switch v := obj.(type) {
		case *Host:
			fmt.Printf("Host[%s]: MAC=%s, IP=%s, Name=%s, Active=%v, Type=%s\n",
				key, v.MACAddress, v.IPAddress, v.HostName, v.Active, v.InterfaceType)
		case *Wifi:
			fmt.Printf("Wifi[%s]: SSID=%s, Channel=%d, Enabled=%v\n",
				key, v.SSID, v.Channel, v.Enabled)
		case *WANPPPConnection:
			fmt.Printf("WAN[%s]: Status=%s, IP=%s, Uptime=%d\n",
				key, v.ConnectionStatus, v.ExternalIP, v.Uptime)
		}
		return nil
	})

	fmt.Printf("\n=== Performance ===\n")
	fmt.Printf("Processing time: %v\n", elapsed)
	fmt.Printf("Avg per item: %v\n", elapsed/time.Duration(len(testData)))

	if stats := fastMapper.GetStats(); stats != nil {
		fmt.Printf("\n%s\n", stats.String())
	}
}

func setupHostRules(m *mapper.FastMapper) {
	hostPatterns := []struct {
		path      string
		field     string
		transform string
	}{
		{"InternetGatewayDevice.LANDevice.*.Hosts.*.MACAddress", "MACAddress", "mac_normalize"},
		{"InternetGatewayDevice.LANDevice.*.Hosts.*.IPAddress", "IPAddress", "ip_validate"},
		{"InternetGatewayDevice.LANDevice.*.Hosts.*.HostName", "HostName", ""},
		{"InternetGatewayDevice.LANDevice.*.Hosts.*.Active", "Active", "bool"},
		{"InternetGatewayDevice.LANDevice.*.Hosts.*.InterfaceType", "InterfaceType", ""},

		{"Device.Hosts.Host.*.PhysAddress", "MACAddress", "mac_normalize"},
		{"Device.Hosts.Host.*.IPAddress", "IPAddress", "ip_validate"},
		{"Device.Hosts.Host.*.HostName", "HostName", ""},
		{"Device.Hosts.Host.*.Active", "Active", "bool"},
	}

	for i, p := range hostPatterns {
		pattern := router.CompilePattern(p.path)
		pattern.Entity = "host"
		pattern.Field = p.field

		var ext extractor.KeyExtractor
		if pattern.Parts != nil && len(pattern.Parts) > 4 {
			ext = &extractor.IndexExtractor{Position: 4, Prefix: "host:"}
		} else {
			ext = &extractor.IndexExtractor{Position: 3, Prefix: "host:"}
		}

		m.AddRule(&mapper.FastRule{
			ID:        fmt.Sprintf("host_%d", i),
			Pattern:   pattern,
			Entity:    "host",
			Field:     p.field,
			Transform: p.transform,
			Extractor: ext,
		})
	}
}

func setupWifiRules(m *mapper.FastMapper) {
	wifiPatterns := []struct {
		path      string
		field     string
		band      string
		transform string
	}{
		{"InternetGatewayDevice.LANDevice.*.WLANConfiguration.1.SSID", "SSID", "2.4GHz", ""},
		{"InternetGatewayDevice.LANDevice.*.WLANConfiguration.1.KeyPassphrase", "Password", "2.4GHz", ""},
		{"InternetGatewayDevice.LANDevice.*.WLANConfiguration.1.Channel", "Channel", "2.4GHz", "int"},
		{"InternetGatewayDevice.LANDevice.*.WLANConfiguration.1.Enable", "Enabled", "2.4GHz", "bool"},

		{"InternetGatewayDevice.LANDevice.*.WLANConfiguration.2.SSID", "SSID", "5GHz", ""},
		{"InternetGatewayDevice.LANDevice.*.WLANConfiguration.2.KeyPassphrase", "Password", "5GHz", ""},
		{"InternetGatewayDevice.LANDevice.*.WLANConfiguration.2.Channel", "Channel", "5GHz", "int"},
		{"InternetGatewayDevice.LANDevice.*.WLANConfiguration.2.Enable", "Enabled", "5GHz", "bool"},

		{"Device.WiFi.AccessPoint.*.SSID", "SSID", "", ""},
		{"Device.WiFi.AccessPoint.*.Security.KeyPassphrase", "Password", "", ""},
		{"Device.WiFi.AccessPoint.*.Enable", "Enabled", "", "bool"},
		{"Device.WiFi.Radio.*.Channel", "Channel", "", "int"},
	}

	for i, p := range wifiPatterns {
		pattern := router.CompilePattern(p.path)
		pattern.Entity = "wifi"
		pattern.Field = p.field

		var ext extractor.KeyExtractor
		if p.band != "" {
			ext = &extractor.StaticExtractor{Value: p.band}
		} else if pattern.Parts != nil && len(pattern.Parts) > 3 {
			ext = &extractor.IndexExtractor{Position: 3, Prefix: "wifi:"}
		} else {
			ext = &extractor.StaticExtractor{Value: "default"}
		}

		m.AddRule(&mapper.FastRule{
			ID:        fmt.Sprintf("wifi_%d", i),
			Pattern:   pattern,
			Entity:    "wifi",
			Field:     p.field,
			Transform: p.transform,
			Extractor: ext,
		})

		if p.band != "" {
			m.AddRule(&mapper.FastRule{
				ID:        fmt.Sprintf("wifi_band_%d", i),
				Pattern:   pattern,
				Entity:    "wifi",
				Field:     "Band",
				Transform: "",
				Extractor: ext,
			})
		}
	}
}

func setupWANRules(m *mapper.FastMapper) {
	wanPatterns := []struct {
		path      string
		field     string
		transform string
	}{
		{"InternetGatewayDevice.WANDevice.*.WANConnectionDevice.*.WANPPPConnection.*.Enable", "Enable", "bool"},
		{"InternetGatewayDevice.WANDevice.*.WANConnectionDevice.*.WANPPPConnection.*.ConnectionStatus", "ConnectionStatus", ""},
		{"InternetGatewayDevice.WANDevice.*.WANConnectionDevice.*.WANPPPConnection.*.ConnectionType", "ConnectionType", ""},
		{"InternetGatewayDevice.WANDevice.*.WANConnectionDevice.*.WANPPPConnection.*.Name", "Name", ""},
		{"InternetGatewayDevice.WANDevice.*.WANConnectionDevice.*.WANPPPConnection.*.Username", "Username", ""},
		{"InternetGatewayDevice.WANDevice.*.WANConnectionDevice.*.WANPPPConnection.*.ExternalIPAddress", "ExternalIP", "ip_validate"},
		{"InternetGatewayDevice.WANDevice.*.WANConnectionDevice.*.WANPPPConnection.*.DNSServers", "DNSServers", ""},
		{"InternetGatewayDevice.WANDevice.*.WANConnectionDevice.*.WANPPPConnection.*.Uptime", "Uptime", "int"},
	}

	for i, p := range wanPatterns {
		pattern := router.CompilePattern(p.path)
		pattern.Entity = "wanppp"
		pattern.Field = p.field

		ext := &extractor.IndexExtractor{Position: 5, Prefix: "wan:"}

		m.AddRule(&mapper.FastRule{
			ID:        fmt.Sprintf("wan_%d", i),
			Pattern:   pattern,
			Entity:    "wanppp",
			Field:     p.field,
			Transform: p.transform,
			Extractor: ext,
		})
	}
}

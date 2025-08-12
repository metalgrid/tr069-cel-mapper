package main

import (
	"fmt"
	"log"

	"github.com/metalgrid/tr069-cel-mapper/pkg/mapper"
	"github.com/metalgrid/tr069-cel-mapper/pkg/registry"
)

type Port struct {
	Name        string
	Status      string
	Utilization float64
}

type Wifi struct {
	SSID    string
	Band    string
	Channel int
}

func main() {
	reg := registry.New()

	reg.MustRegister("Port", func() any { return &Port{} })
	reg.MustRegister("Wifi", func() any { return &Wifi{} })

	m := mapper.New(reg, mapper.WithMetrics())

	if err := m.LoadRulesFromFile("../configs/example-rules.yaml"); err != nil {
		log.Fatalf("Failed to load rules: %v", err)
	}

	lines := [][2]string{
		{"InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.SSID", "Home"},
		{"InternetGatewayDevice.LANDevice.1.LANEthernetInterfaceConfig.1.X_Vendor.Utilization", "80%"},
		{"InternetGatewayDevice.LANDevice.1.LANEthernetInterfaceConfig.1.Name", "LAN1"},
		{"InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.Channel", "11"},
		{"InternetGatewayDevice.LANDevice.1.LANEthernetInterfaceConfig.1.Status", "Up"},
		{"InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.OperatingFrequencyBand", "5GHz"},
		{"InternetGatewayDevice.LANDevice.1.WLANConfiguration.2.SSID", "Guest"},
		{"InternetGatewayDevice.LANDevice.1.WLANConfiguration.2.OperatingFrequencyBand", "2.4GHz"},
		{"InternetGatewayDevice.LANDevice.1.WLANConfiguration.2.Channel", "6"},
		{"InternetGatewayDevice.LANDevice.1.LANEthernetInterfaceConfig.2.Name", "LAN2"},
		{"InternetGatewayDevice.LANDevice.1.LANEthernetInterfaceConfig.2.Status", "Down"},
	}

	if err := m.ProcessBatch(lines); err != nil {
		log.Fatalf("Failed to process batch: %v", err)
	}

	store := m.GetStore()
	err := store.ForEach(func(target, key string, obj any) error {
		switch v := obj.(type) {
		case *Port:
			fmt.Printf("Port[%s]: Name=%s, Status=%s, Utilization=%.1f%%\n",
				key, v.Name, v.Status, v.Utilization)
		case *Wifi:
			fmt.Printf("Wifi[%s]: SSID=%s, Band=%s, Channel=%d\n",
				key, v.SSID, v.Band, v.Channel)
		}
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to iterate store: %v", err)
	}

	if metrics := m.GetMetrics(); metrics != nil {
		fmt.Printf("\nMetrics:\n")
		fmt.Printf("  Processed Lines: %d\n", metrics.ProcessedLines)
		fmt.Printf("  Matched Rules: %d\n", metrics.MatchedRules)
		fmt.Printf("  Failed Rules: %d\n", metrics.FailedRules)
		fmt.Printf("  Processing Time: %v\n", metrics.ProcessingTime)
	}
}

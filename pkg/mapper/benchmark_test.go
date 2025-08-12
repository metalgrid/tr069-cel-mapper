package mapper

import (
	"fmt"
	"testing"

	"github.com/metalgrid/tr069-cel-mapper/pkg/extractor"
	"github.com/metalgrid/tr069-cel-mapper/pkg/registry"
	"github.com/metalgrid/tr069-cel-mapper/pkg/router"
	"github.com/metalgrid/tr069-cel-mapper/pkg/transform"
)

type TestHost struct {
	MACAddress string
	IPAddress  string
	HostName   string
	Active     bool
}

type TestWifi struct {
	SSID     string
	Password string
	Channel  int
	Enabled  bool
}

func BenchmarkFastMapper(b *testing.B) {
	reg := registry.New()
	reg.MustRegister("host", func() any { return &TestHost{} })
	reg.MustRegister("wifi", func() any { return &TestWifi{} })

	mapper := NewFast(reg, WithFastStats())

	hostMACPattern := router.CompilePattern("InternetGatewayDevice.LANDevice.*.Hosts.*.MACAddress")
	hostMACPattern.Entity = "host"
	hostMACPattern.Field = "MACAddress"
	mapper.AddRule(&FastRule{
		ID:        "host_mac",
		Pattern:   hostMACPattern,
		Entity:    "host",
		Field:     "MACAddress",
		Transform: "mac_normalize",
		Extractor: extractor.CompileExtractor("path[4]"),
	})

	hostIPPattern := router.CompilePattern("InternetGatewayDevice.LANDevice.*.Hosts.*.IPAddress")
	hostIPPattern.Entity = "host"
	hostIPPattern.Field = "IPAddress"
	mapper.AddRule(&FastRule{
		ID:        "host_ip",
		Pattern:   hostIPPattern,
		Entity:    "host",
		Field:     "IPAddress",
		Transform: "ip_validate",
		Extractor: extractor.CompileExtractor("path[4]"),
	})

	testData := [][2]string{
		{"InternetGatewayDevice.LANDevice.1.Hosts.1.MACAddress", "AA:BB:CC:DD:EE:FF"},
		{"InternetGatewayDevice.LANDevice.1.Hosts.1.IPAddress", "192.168.1.100"},
		{"InternetGatewayDevice.LANDevice.1.Hosts.2.MACAddress", "11:22:33:44:55:66"},
		{"InternetGatewayDevice.LANDevice.1.Hosts.2.IPAddress", "192.168.1.101"},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		for _, data := range testData {
			mapper.Process(data[0], data[1])
		}
	}

	b.StopTimer()
	stats := mapper.GetStats()
	if stats != nil {
		b.Logf("Performance: %s", stats.String())
	}
}

func BenchmarkFastMapperParallel(b *testing.B) {
	reg := registry.New()
	reg.MustRegister("host", func() any { return &TestHost{} })

	mapper := NewFast(reg, WithFastStats())

	pattern := router.CompilePattern("*.Hosts.*.MACAddress")
	pattern.Entity = "host"
	pattern.Field = "MACAddress"
	mapper.AddRule(&FastRule{
		ID:        "host_mac",
		Pattern:   pattern,
		Entity:    "host",
		Field:     "MACAddress",
		Transform: "mac_normalize",
		Extractor: extractor.CompileExtractor("value"),
	})

	paths := make([]string, 100)
	for i := 0; i < 100; i++ {
		paths[i] = fmt.Sprintf("Device.Hosts.Host.%d.MACAddress", i)
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			path := paths[i%len(paths)]
			mapper.Process(path, "AA:BB:CC:DD:EE:FF")
			i++
		}
	})

	b.StopTimer()
	stats := mapper.GetStats()
	if stats != nil {
		b.Logf("Performance: %s", stats.String())
	}
}

func BenchmarkRouterOnly(b *testing.B) {
	r := router.New()

	patterns := []string{
		"InternetGatewayDevice.LANDevice.*.Hosts.*.MACAddress",
		"InternetGatewayDevice.LANDevice.*.Hosts.*.IPAddress",
		"Device.Hosts.Host.*.MACAddress",
		"Device.Hosts.Host.*.IPAddress",
		"*.WiFi.AccessPoint.*.SSID",
		"*.WiFi.AccessPoint.*.Security.KeyPassphrase",
	}

	for _, p := range patterns {
		compiled := router.CompilePattern(p)
		r.AddPattern(compiled)
	}

	testPaths := []string{
		"InternetGatewayDevice.LANDevice.1.Hosts.1.MACAddress",
		"Device.Hosts.Host.42.IPAddress",
		"Something.WiFi.AccessPoint.2.SSID",
		"NoMatch.Path.Here",
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		path := testPaths[i%len(testPaths)]
		r.Route(path)
	}
}

func BenchmarkExtractorOnly(b *testing.B) {
	extractors := []extractor.KeyExtractor{
		extractor.CompileExtractor("path[4]"),
		extractor.CompileExtractor("value"),
		extractor.CompileExtractor("host:path[4]"),
	}

	path := "InternetGatewayDevice.LANDevice.1.Hosts.42.MACAddress"
	value := "AA:BB:CC:DD:EE:FF"

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ext := extractors[i%len(extractors)]
		ext.Extract(path, value)
	}
}

func BenchmarkTransformOnly(b *testing.B) {
	transformer := transform.NewFastTransform()

	testData := []struct {
		transform string
		value     string
	}{
		{"mac_normalize", "AA:BB:CC:DD:EE:FF"},
		{"ip_validate", "192.168.1.100"},
		{"bool", "true"},
		{"int", "42"},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		data := testData[i%len(testData)]
		transformer.Transform(data.transform, data.value)
	}
}

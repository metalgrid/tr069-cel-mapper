# TR-069 CEL Mapper Usage Guide

## Installation

```bash
go get github.com/metalgrid/tr069-cel-mapper
```

## Choose Your Performance Mode

The library offers two modes:

1. **Fast Mode** (Recommended) - String operations, ~1.2μs per operation, 800K+ msgs/sec
2. **Standard Mode** - CEL-based, flexible, ~10μs per operation

## Fast Mode Usage (High Throughput)

### 1. Define Your Domain Types

```go
type Host struct {
    MACAddress string
    IPAddress  string
    HostName   string
    Active     bool
}

type Wifi struct {
    SSID     string
    Password string
    Channel  int
    Enabled  bool
}
```

### 2. Set Up the Fast Mapper

```go
import (
    "github.com/metalgrid/tr069-cel-mapper/pkg/mapper"
    "github.com/metalgrid/tr069-cel-mapper/pkg/registry"
    "github.com/metalgrid/tr069-cel-mapper/pkg/router"
    "github.com/metalgrid/tr069-cel-mapper/pkg/extractor"
)

// Create registry and register types
reg := registry.New()
reg.MustRegister("host", func() any { return &Host{} })
reg.MustRegister("wifi", func() any { return &Wifi{} })

// Create fast mapper with stats
m := mapper.NewFast(reg, mapper.WithFastStats())
```

### 3. Add Rules Programmatically

```go
// Pattern for: InternetGatewayDevice.LANDevice.*.Hosts.*.MACAddress
pattern := router.CompilePattern("InternetGatewayDevice.LANDevice.*.Hosts.*.MACAddress")

m.AddRule(&mapper.FastRule{
    ID:        "host_mac",
    Pattern:   pattern,
    Entity:    "host",
    Field:     "MACAddress",
    Transform: "mac_normalize",  // Built-in transform
    Extractor: &extractor.IndexExtractor{
        Position: 4,              // Extract instance from path[4]
        Prefix:   "host:",       // Create key like "host:1"
    },
})
```

### 4. Process Data

```go
// Single message
m.Process("InternetGatewayDevice.LANDevice.1.Hosts.1.MACAddress", "AA:BB:CC:DD:EE:FF")

// Batch processing (uses parallel workers)
items := [][2]string{
    {"path1", "value1"},
    {"path2", "value2"},
    // ...
}
m.ProcessBatch(items)
```

### 5. Access Results

```go
store := m.GetStore()
store.ForEach(func(target, key string, obj any) error {
    switch v := obj.(type) {
    case *Host:
        fmt.Printf("Host[%s]: %+v\n", key, v)
    case *Wifi:
        fmt.Printf("Wifi[%s]: %+v\n", key, v)
    }
    return nil
})
```

## Standard Mode (CEL-Based)

For complex transformations that need CEL expressions:

### 1. Create YAML Configuration

```yaml
# rules.yaml
version: "1.0"
rules:
  - name: complex_rule
    target: Host
    route: 'path.matches(".*\\.Hosts\\..*") && path.contains("Device")'
    entity_key: 'path.split(".")[2] + ":" + value.substring(0, 2)'
    fields:
      - name: MACAddress
        when: 'path.endsWith("MACAddress")'
        value: 'value.replace(":", "").lower()'
```

### 2. Load and Use

```go
import "github.com/metalgrid/tr069-cel-mapper/pkg/mapper"

m := mapper.New(reg)
m.LoadRulesFromFile("rules.yaml")
m.Process(path, value)
```

## TR-069/TR-181 Specific Features

### Common Path Patterns

```go
// TR-069 patterns
"InternetGatewayDevice.LANDevice.*.Hosts.*.MACAddress"
"InternetGatewayDevice.LANDevice.*.WLANConfiguration.*.SSID"
"InternetGatewayDevice.WANDevice.*.WANConnectionDevice.*.WANPPPConnection.*.Enable"

// TR-181 patterns  
"Device.Hosts.Host.*.PhysAddress"
"Device.WiFi.AccessPoint.*.SSID"
"Device.WiFi.Radio.*.Channel"
```

### Key Extractors

Several built-in extractors for entity key generation:

```go
// Extract from path index (common for TR-069)
&extractor.IndexExtractor{Position: 4, Prefix: "host:"}

// Use the value as key (for MAC addresses)
&extractor.ValueExtractor{}

// Static key (for singleton entities)
&extractor.StaticExtractor{Value: "singleton"}

// Composite key (for complex entities)
&extractor.CompositeExtractor{
    Parts: []KeyExtractor{
        &extractor.IndexExtractor{Position: 2},
        &extractor.IndexExtractor{Position: 4},
    },
    Sep: ":",
}
```

### Built-in Transforms

TR-069 specific transforms:

- `mac_normalize` - Normalize MAC addresses (AA:BB:CC:DD:EE:FF → aa:bb:cc:dd:ee:ff)
- `ip_validate` - Validate and normalize IP addresses
- `bool` - Convert TR-069 booleans ("true", "1", "yes", "enabled")
- `int` - Convert to integer (handles comma-separated numbers)
- `float` - Convert to float (handles percentages)

## Performance Optimization

### Enable Object Pooling

```go
// Objects are automatically pooled in Fast mode
m := mapper.NewFast(reg, mapper.WithFastStats())
```

### Batch Processing

```go
// Process 1000+ items efficiently
m.ProcessBatch(items) // Automatically uses parallel workers
```

### Performance Monitoring

```go
stats := m.GetStats()
fmt.Println(stats.String())
// Output: Stats: 1000 lines, 950 matched | Memory: 10 allocs, 990 reused (99% reuse) | Avg: 1200ns
```

## Migration from Legacy Config

### Legacy Format
```yaml
- path: "*.Hosts.*.MACAddress"
  field: "mac_address"
  entity: "host"
  transform: "mac_normalize"
```

### Convert to FastRule
```go
pattern := router.CompilePattern("*.Hosts.*.MACAddress")
rule := &mapper.FastRule{
    ID:        "legacy_host_mac",
    Pattern:   pattern,
    Entity:    "host",
    Field:     "MACAddress",
    Transform: "mac_normalize",
    Extractor: &extractor.IndexExtractor{Position: 2},
}
```

## Performance Benchmarks

- **Single-threaded**: 800K+ messages/second
- **Multi-threaded (20 cores)**: 74M+ operations/second
- **Memory**: Near-zero allocations after warmup
- **Latency**: ~1.2μs average per message

## Best Practices

1. **Use Fast Mode** for production TR-069 processing
2. **Pre-compile patterns** at startup
3. **Batch process** when handling bulk data
4. **Monitor performance** with built-in stats
5. **Use appropriate extractors** based on your TR-069 path structure

## Thread Safety

Both mappers are thread-safe:

```go
// Safe for concurrent use
go m.Process(path1, value1)
go m.Process(path2, value2)
```

## Context Support

For cancellation and timeouts:

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

m.ProcessContext(ctx, path, value)
m.ProcessBatchContext(ctx, items)
```
# CEL Mapper

A production-grade Go library for mapping unstructured key-value data to strongly-typed domain objects using Google's Common Expression Language (CEL).

## Features

- **Declarative Rule Configuration**: Define mapping rules in YAML files
- **Type-Safe Domain Registration**: Register domain types at compile time
- **Flexible Expression Evaluation**: Use CEL expressions for routing, key extraction, and value transformation
- **Efficient Object Construction**: Build domain objects incrementally from scattered data
- **Thread-Safe Operations**: Concurrent-safe rule processing and storage
- **Comprehensive Error Handling**: Detailed error reporting with context
- **Performance Metrics**: Built-in metrics tracking for monitoring
- **Extensible Architecture**: Easy to extend with custom types and functions

## Installation

```bash
go get github.com/example/cel-mapper
```

## Quick Start

### 1. Define Your Domain Types

```go
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
```

### 2. Create Mapping Rules (YAML)

```yaml
version: "1.0"
rules:
  - name: port_rule
    target: Port
    route: 'path.matches("^InternetGatewayDevice\\.LANDevice\\.\\d+\\.LANEthernetInterfaceConfig\\.\\d+\\..*")'
    entity_key: '"port:" + path.split(".")[4]'
    fields:
      - name: Name
        when: 'path.endsWith(".Name")'
        value: value
      - name: Status
        when: 'path.endsWith(".Status")'
        value: value
      - name: Utilization
        when: 'path.endsWith(".X_Vendor.Utilization")'
        value: 'double(value.replace("%", ""))'
```

### 3. Use the Library

```go
package main

import (
    "fmt"
    "log"
    
    "github.com/example/cel-mapper/pkg/mapper"
    "github.com/example/cel-mapper/pkg/registry"
)

func main() {
    // Create registry and register types
    reg := registry.New()
    reg.MustRegister("Port", func() any { return &Port{} })
    reg.MustRegister("Wifi", func() any { return &Wifi{} })
    
    // Create mapper with options
    m := mapper.New(reg, mapper.WithMetrics())
    
    // Load rules from YAML
    if err := m.LoadRulesFromFile("rules.yaml"); err != nil {
        log.Fatal(err)
    }
    
    // Process data
    m.Process("InternetGatewayDevice.LANDevice.1.LANEthernetInterfaceConfig.1.Name", "LAN1")
    m.Process("InternetGatewayDevice.LANDevice.1.LANEthernetInterfaceConfig.1.Status", "Up")
    
    // Access results
    store := m.GetStore()
    store.ForEach(func(target, key string, obj any) error {
        fmt.Printf("%s[%s]: %+v\n", target, key, obj)
        return nil
    })
}
```

## Architecture

### Core Components

1. **Registry** (`pkg/registry`): Type registration and reflection-based setter generation
2. **Loader** (`pkg/loader`): YAML configuration loading and validation
3. **Builder** (`pkg/builder`): CEL expression compilation and rule construction
4. **Mapper** (`pkg/mapper`): Main processing engine with rule evaluation
5. **Types** (`pkg/types`): Core type definitions and interfaces

### Processing Flow

1. **Registration**: Register domain types with factory functions
2. **Rule Loading**: Load and compile CEL expressions from YAML
3. **Data Processing**: For each input line:
   - Evaluate route expression to find matching rule
   - Extract entity key using CEL expression
   - Create/update domain object in store
   - Apply field mappings based on conditions
4. **Result Access**: Query store for constructed objects

## Rule Configuration

### Rule Structure

```yaml
version: "1.0"
rules:
  - name: <rule_name>
    target: <registered_type_name>
    route: <cel_expression_returning_bool>
    entity_key: <cel_expression_returning_string>
    fields:
      - name: <field_name>
        when: <cel_expression_returning_bool>
        value: <cel_expression_returning_value>
```

### Available CEL Variables

- `path`: The input path/key (string)
- `value`: The input value (string)

### CEL Functions

All standard CEL functions plus string extensions:
- `matches()`, `contains()`, `startsWith()`, `endsWith()`
- `split()`, `replace()`, `trim()`, `lower()`, `upper()`
- Type conversions: `int()`, `double()`, `string()`, `bool()`

## Advanced Usage

### Custom Error Handling

```go
m := mapper.New(reg, mapper.WithErrorHandler(func(err error) {
    log.Printf("Processing error: %v", err)
}))
```

### Batch Processing

```go
items := [][2]string{
    {"path1", "value1"},
    {"path2", "value2"},
}
m.ProcessBatch(items)
```

### Context Support

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
m.ProcessWithContext(ctx, path, value)
```

### Metrics

```go
m := mapper.New(reg, mapper.WithMetrics())
// ... process data ...
metrics := m.GetMetrics()
fmt.Printf("Processed: %d lines\n", metrics.ProcessedLines)
fmt.Printf("Matched: %d rules\n", metrics.MatchedRules)
```

## Performance Considerations

- Rules are evaluated sequentially; place most common rules first
- CEL expressions are compiled once during rule loading
- Object creation uses factory functions for efficiency
- Reflection-based setters are cached per type
- Thread-safe for concurrent processing

## License

MIT License - See LICENSE file for details
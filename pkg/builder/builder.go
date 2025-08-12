package builder

import (
	"fmt"
	"sync"

	"github.com/example/cel-mapper/pkg/loader"
	"github.com/example/cel-mapper/pkg/registry"
	"github.com/example/cel-mapper/pkg/types"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"
)

type Builder struct {
	registry   *registry.Registry
	envOptions []cel.EnvOption
	variables  map[string]*cel.Type
	functions  []cel.EnvOption
	mu         sync.RWMutex
}

func New(reg *registry.Registry) *Builder {
	return &Builder{
		registry:  reg,
		variables: make(map[string]*cel.Type),
		functions: []cel.EnvOption{ext.Strings()},
	}
}

func (b *Builder) WithVariable(name string, celType *cel.Type) *Builder {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.variables[name] = celType
	return b
}

func (b *Builder) WithFunction(opt cel.EnvOption) *Builder {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.functions = append(b.functions, opt)
	return b
}

func (b *Builder) WithStandardVariables() *Builder {
	return b.
		WithVariable("path", cel.StringType).
		WithVariable("value", cel.StringType)
}

func (b *Builder) BuildFromConfig(config *types.RulesConfig) ([]*types.CompiledRule, error) {
	env, err := b.createEnvironment()
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %w", err)
	}

	rules := make([]*types.CompiledRule, 0, len(config.Rules))
	for _, ruleConfig := range config.Rules {
		rule, err := b.buildRule(env, &ruleConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to build rule %s: %w", ruleConfig.Name, err)
		}
		rules = append(rules, rule)
	}

	return rules, nil
}

func (b *Builder) BuildFromFile(filename string) ([]*types.CompiledRule, error) {
	config, err := loader.LoadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to load config file: %w", err)
	}
	return b.BuildFromConfig(config)
}

func (b *Builder) BuildFromString(content string) ([]*types.CompiledRule, error) {
	config, err := loader.LoadString(content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	return b.BuildFromConfig(config)
}

func (b *Builder) createEnvironment() (*cel.Env, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	options := make([]cel.EnvOption, 0, len(b.variables)+len(b.functions)+len(b.envOptions))

	for name, celType := range b.variables {
		options = append(options, cel.Variable(name, celType))
	}

	options = append(options, b.functions...)
	options = append(options, b.envOptions...)

	return cel.NewEnv(options...)
}

func (b *Builder) buildRule(env *cel.Env, config *types.RuleConfig) (*types.CompiledRule, error) {
	typeInfo, err := b.registry.Get(config.Target)
	if err != nil {
		return nil, fmt.Errorf("target type %s not registered: %w", config.Target, err)
	}

	routeProg, err := b.compileExpression(env, config.Route, "route")
	if err != nil {
		return nil, err
	}

	keyProg, err := b.compileExpression(env, config.EntityKey, "entity_key")
	if err != nil {
		return nil, err
	}

	fields := make([]types.CompiledFieldRule, 0, len(config.Fields))
	for _, fieldConfig := range config.Fields {
		field, err := b.buildField(env, &fieldConfig, typeInfo)
		if err != nil {
			return nil, fmt.Errorf("failed to build field %s: %w", fieldConfig.Name, err)
		}
		fields = append(fields, *field)
	}

	return &types.CompiledRule{
		Name:      config.Name,
		Target:    config.Target,
		Route:     routeProg,
		EntityKey: keyProg,
		Fields:    fields,
		Factory:   typeInfo.Factory,
	}, nil
}

func (b *Builder) buildField(env *cel.Env, config *types.FieldMapping, typeInfo *registry.TypeInfo) (*types.CompiledFieldRule, error) {
	whenProg, err := b.compileExpression(env, config.When, fmt.Sprintf("field[%s].when", config.Name))
	if err != nil {
		return nil, err
	}

	valueProg, err := b.compileExpression(env, config.Value, fmt.Sprintf("field[%s].value", config.Name))
	if err != nil {
		return nil, err
	}

	setter, ok := typeInfo.Setters[config.Name]
	if !ok {
		return nil, fmt.Errorf("field %s not found in type %s", config.Name, typeInfo.Type.Name())
	}

	return &types.CompiledFieldRule{
		Name:   config.Name,
		When:   whenProg,
		Value:  valueProg,
		Setter: setter,
	}, nil
}

func (b *Builder) compileExpression(env *cel.Env, expr string, context string) (cel.Program, error) {
	ast, issues := env.Parse(expr)
	if issues.Err() != nil {
		return nil, fmt.Errorf("failed to parse %s expression '%s': %w", context, expr, issues.Err())
	}

	checked, issues := env.Check(ast)
	if issues.Err() != nil {
		return nil, fmt.Errorf("failed to check %s expression '%s': %w", context, expr, issues.Err())
	}

	prog, err := env.Program(checked)
	if err != nil {
		return nil, fmt.Errorf("failed to compile %s expression '%s': %w", context, expr, err)
	}

	return prog, nil
}

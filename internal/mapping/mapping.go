// Package mapping supplies the same bounded CEL environment to preview and execution.
package mapping

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"time"

	"cel.dev/cel-go/cel"
	"google.golang.org/protobuf/types/known/structpb"
)

var ErrExpression = errors.New("invalid or over-budget expression")

func environment() (*cel.Env, error) { return cel.NewEnv(cel.Variable("input", cel.DynType)) }
func Validate(expression string) error {
	if len(expression) == 0 || len(expression) > 2048 {
		return ErrExpression
	}
	env, err := environment()
	if err != nil {
		return err
	}
	_, issues := env.Compile(expression)
	if issues.Err() != nil {
		return ErrExpression
	}
	return nil
}
func Evaluate(ctx context.Context, expression string, input any) (any, error) {
	encodedInput, inputErr := json.Marshal(input)
	if inputErr != nil || len(encodedInput) > 8192 {
		return nil, ErrExpression
	}
	if err := Validate(expression); err != nil {
		return nil, err
	}
	env, err := environment()
	if err != nil {
		return nil, err
	}
	ast, issues := env.Compile(expression)
	if issues.Err() != nil {
		return nil, ErrExpression
	}
	program, err := env.Program(ast, cel.CostLimit(10000), cel.InterruptCheckFrequency(100))
	if err != nil {
		return nil, ErrExpression
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	out, _, err := program.ContextEval(ctx, map[string]any{"input": jsonNumbers(input)})
	if err != nil {
		return nil, ErrExpression
	}
	native, err := out.ConvertToNative(reflect.TypeOf(&structpb.Value{}))
	if err != nil {
		return nil, ErrExpression
	}
	value, ok := native.(*structpb.Value)
	if !ok {
		return nil, ErrExpression
	}
	result := value.AsInterface()
	encoded, err := json.Marshal(result)
	if err != nil || len(encoded) > 8192 {
		return nil, ErrExpression
	}
	return result, nil
}

// JSON does not distinguish integers from doubles. Preserve whole portable values as
// CEL integers so ordinary arithmetic agrees with integer literals in expressions.
func jsonNumbers(value any) any {
	switch v := value.(type) {
	case float64:
		if math.Trunc(v) == v && math.Abs(v) <= 9007199254740991 {
			return int64(v)
		}
		return v
	case map[string]any:
		result := make(map[string]any, len(v))
		for k, item := range v {
			result[k] = jsonNumbers(item)
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = jsonNumbers(item)
		}
		return result
	default:
		return value
	}
}

package calculator

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"awaken/src/tool"
)

type CalculatorTool struct{}

var _ tool.LegacySingleValueInput = (*CalculatorTool)(nil)

func (t *CalculatorTool) Name() string {
	return "calculator"
}

func (t *CalculatorTool) Description() string {
	return "执行纯数学表达式计算，支持加减乘除、幂运算、平方根等基本数学运算；不执行代码、不读取文件、不做表格/文档分析"
}

func (t *CalculatorTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{
			Name:        "expression",
			Description: "要计算的纯数学表达式，例如 1+2*3 或 sqrt(9)",
			Type:        "string",
			Required:    true,
		},
	}
}

func (t *CalculatorTool) Definition() tool.ToolDefinition {
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name:        "calculator",
		DisplayName: "Calculator",
		Description: t.Description(),
		Purpose:     "确定性计算一个受支持的纯数学表达式，不执行代码或读取外部数据。",
		WhenToUse: []string{
			"需要精确计算加减乘除、幂或平方根等基础表达式时",
			"结果只依赖显式数字和数学运算时",
		},
		WhenNotToUse: []string{
			"分析表格、文件或业务数据时先使用对应读取工具",
			"执行代码、脚本、单位/汇率实时换算或联网查询时使用匹配工具",
		},
		OutputSchema: tool.ToolParam{Type: "object", Properties: map[string]tool.ToolParam{
			"expression": {Type: "string", Description: "实际求值表达式"},
			"result":     {Type: "string", Description: "格式化后的确定性结果"},
		}},
		SideEffects: []tool.ToolSideEffect{{Kind: tool.SideEffectNone, Description: "只在内存中求值，不修改状态。", Reversibility: tool.ReversibilityNotApplicable}},
		Examples:    []tool.ToolExample{{Scenario: "计算基础表达式", Input: map[string]any{"expression": "sqrt(81)+2^3"}, Outcome: "返回 17 及原表达式"}},
		FailureModes: []tool.ToolFailureMode{
			{Code: "calculator_expression_required", Cause: "expression 为空", Retryable: false, Recovery: []string{"提供纯数学表达式"}},
			{Code: "calculator_invalid_expression", Cause: "表达式语法或运算不受支持", Retryable: false, Recovery: []string{"改用支持的基础运算"}},
		},
		Annotations: tool.ToolAnnotations{
			ReadOnly: true, Destructive: false, OpenWorld: false,
			CapabilityClass: tool.CapabilityLocalTransform, TrustedOrigin: tool.TrustedOriginBuiltin,
			FileBridgeScope: tool.FileBridgeNone, NetworkScope: tool.NetworkScopeNone,
		},
	})
}

func (t *CalculatorTool) IsReadOnly() bool {
	return true
}

// UnwrapSingleParameterInput tells the runtime that this one-argument
// compatibility tool evaluates the string value itself rather than a JSON
// object wrapper.
func (t *CalculatorTool) UnwrapSingleParameterInput() bool { return true }

func (t *CalculatorTool) Execute(ctx context.Context, input string) (string, error) {
	outcome, err := t.ExecuteOutcome(ctx, input, nil)
	if err != nil {
		return outcome.Content, err
	}
	if outcome.IsError() {
		return "", fmt.Errorf("%s", outcome.ErrorMessage)
	}
	return outcome.Content, nil
}

// ExecuteOutcome keeps malformed input and unsupported expressions in the
// semantic result channel so the agent can correct its invocation without
// treating a calculator mistake as a transport outage.
func (t *CalculatorTool) ExecuteOutcome(_ context.Context, input string, _ tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	input = strings.TrimSpace(input)
	if input == "" {
		outcome := tool.FailedOutcome("表达式不能为空", "calculator_expression_required", tool.ErrorCategoryValidation, false)
		outcome.RecoveryOptions = []string{"provide_math_expression"}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}

	result, err := evaluateExpression(input)
	if err != nil {
		message := fmt.Sprintf("计算失败: %v", err)
		outcome := tool.FailedOutcome(message, "calculator_invalid_expression", tool.ErrorCategoryDomain, false)
		outcome.RecoveryOptions = []string{"correct_math_expression", "use_supported_basic_operations"}
		outcome.Diagnostic = map[string]any{"expression": input}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}

	content := fmt.Sprintf("%.10g", result)
	outcome := tool.SucceededOutcome(content)
	outcome.Diagnostic = map[string]any{"expression": input, "result": content}
	outcome.Details = map[string]any{"expression": input, "result": content}
	return outcome.WithTiming(started, time.Now().UTC()), nil
}

func evaluateExpression(expr string) (float64, error) {
	parser := expressionParser{input: expr}
	result, err := parser.parseExpression()
	if err != nil {
		return 0, err
	}
	parser.skipWhitespace()
	if parser.position != len(parser.input) {
		return 0, fmt.Errorf("无法识别位置 %d 附近的输入", parser.position+1)
	}
	if math.IsNaN(result) || math.IsInf(result, 0) {
		return 0, fmt.Errorf("计算结果不是有限数值")
	}
	return result, nil
}

// expressionParser implements the calculator's documented grammar with
// conventional precedence: parentheses/functions, exponentiation,
// multiplication/division, then addition/subtraction. Exponentiation is
// right-associative, so 2^3^2 is parsed as 2^(3^2).
type expressionParser struct {
	input    string
	position int
}

func (p *expressionParser) parseExpression() (float64, error) {
	value, err := p.parseTerm()
	if err != nil {
		return 0, err
	}
	for {
		p.skipWhitespace()
		switch {
		case p.consume('+'):
			right, err := p.parseTerm()
			if err != nil {
				return 0, err
			}
			value += right
		case p.consume('-'):
			right, err := p.parseTerm()
			if err != nil {
				return 0, err
			}
			value -= right
		default:
			return value, nil
		}
	}
}

func (p *expressionParser) parseTerm() (float64, error) {
	value, err := p.parseUnary()
	if err != nil {
		return 0, err
	}
	for {
		p.skipWhitespace()
		switch {
		case p.consume('*'):
			right, err := p.parseUnary()
			if err != nil {
				return 0, err
			}
			value *= right
		case p.consume('/'):
			right, err := p.parseUnary()
			if err != nil {
				return 0, err
			}
			if right == 0 {
				return 0, fmt.Errorf("除零错误")
			}
			value /= right
		default:
			return value, nil
		}
	}
}

func (p *expressionParser) parseUnary() (float64, error) {
	p.skipWhitespace()
	if p.consume('+') {
		return p.parseUnary()
	}
	if p.consume('-') {
		value, err := p.parseUnary()
		return -value, err
	}
	return p.parsePower()
}

func (p *expressionParser) parsePower() (float64, error) {
	base, err := p.parsePrimary()
	if err != nil {
		return 0, err
	}
	p.skipWhitespace()
	if !p.consume('^') {
		return base, nil
	}
	exponent, err := p.parseUnary()
	if err != nil {
		return 0, err
	}
	return math.Pow(base, exponent), nil
}

func (p *expressionParser) parsePrimary() (float64, error) {
	p.skipWhitespace()
	if p.consumeString("sqrt") {
		p.skipWhitespace()
		if !p.consume('(') {
			return 0, fmt.Errorf("sqrt 后需要括号")
		}
		value, err := p.parseExpression()
		if err != nil {
			return 0, err
		}
		p.skipWhitespace()
		if !p.consume(')') {
			return 0, fmt.Errorf("sqrt 缺少右括号")
		}
		if value < 0 {
			return 0, fmt.Errorf("平方根参数不能为负数")
		}
		return math.Sqrt(value), nil
	}
	if p.consume('(') {
		value, err := p.parseExpression()
		if err != nil {
			return 0, err
		}
		p.skipWhitespace()
		if !p.consume(')') {
			return 0, fmt.Errorf("缺少右括号")
		}
		return value, nil
	}
	return p.parseNumber()
}

func (p *expressionParser) parseNumber() (float64, error) {
	p.skipWhitespace()
	start := p.position
	seenDigit := false
	for p.position < len(p.input) {
		ch := p.input[p.position]
		if ch >= '0' && ch <= '9' {
			seenDigit = true
			p.position++
			continue
		}
		if ch == '.' {
			p.position++
			continue
		}
		break
	}
	if !seenDigit {
		return 0, fmt.Errorf("位置 %d 需要数字或表达式", start+1)
	}
	value, err := strconv.ParseFloat(p.input[start:p.position], 64)
	if err != nil {
		return 0, fmt.Errorf("无效数字 %q: %w", p.input[start:p.position], err)
	}
	return value, nil
}

func (p *expressionParser) skipWhitespace() {
	for p.position < len(p.input) {
		switch p.input[p.position] {
		case ' ', '\t', '\n', '\r':
			p.position++
		default:
			return
		}
	}
}

func (p *expressionParser) consume(ch byte) bool {
	if p.position >= len(p.input) || p.input[p.position] != ch {
		return false
	}
	p.position++
	return true
}

func (p *expressionParser) consumeString(value string) bool {
	if !strings.HasPrefix(p.input[p.position:], value) {
		return false
	}
	p.position += len(value)
	return true
}

func init() {
	tool.MustRegisterToolWithMode(&CalculatorTool{}, tool.DefinitionValidationStrictBuiltin)
}

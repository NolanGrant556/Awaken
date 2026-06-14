package calculator

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"awaken/src/tool"
)

type CalculatorTool struct{}

func (t *CalculatorTool) Name() string {
	return "calculator"
}

func (t *CalculatorTool) Description() string {
	return "用于执行数学计算，支持加减乘除、幂运算、平方根等基本数学运算"
}

func (t *CalculatorTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{
			Name:        "expression",
			Description: "要计算的数学表达式",
			Type:        "string",
			Required:    true,
		},
	}
}

func (t *CalculatorTool) Execute(ctx context.Context, input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("表达式不能为空")
	}

	result, err := evaluateExpression(input)
	if err != nil {
		return "", fmt.Errorf("计算失败: %v", err)
	}

	return fmt.Sprintf("%.10g", result), nil
}

func evaluateExpression(expr string) (float64, error) {
	expr = strings.ReplaceAll(expr, " ", "")

	if strings.Contains(expr, "^") {
		parts := strings.SplitN(expr, "^", 2)
		base, err := evaluateExpression(parts[0])
		if err != nil {
			return 0, err
		}
		exp, err := evaluateExpression(parts[1])
		if err != nil {
			return 0, err
		}
		return math.Pow(base, exp), nil
	}

	if strings.Contains(expr, "sqrt(") && strings.HasSuffix(expr, ")") {
		inner := expr[5 : len(expr)-1]
		val, err := evaluateExpression(inner)
		if err != nil {
			return 0, err
		}
		return math.Sqrt(val), nil
	}

	if strings.Contains(expr, "+") {
		parts := strings.Split(expr, "+")
		var sum float64
		for _, part := range parts {
			val, err := evaluateExpression(part)
			if err != nil {
				return 0, err
			}
			sum += val
		}
		return sum, nil
	}

	if strings.Contains(expr, "-") && !strings.HasPrefix(expr, "-") {
		parts := strings.Split(expr, "-")
		if len(parts) == 0 {
			return 0, fmt.Errorf("无效表达式")
		}
		val, err := evaluateExpression(parts[0])
		if err != nil {
			return 0, err
		}
		for i := 1; i < len(parts); i++ {
			sub, err := evaluateExpression(parts[i])
			if err != nil {
				return 0, err
			}
			val -= sub
		}
		return val, nil
	}

	if strings.Contains(expr, "*") {
		parts := strings.Split(expr, "*")
		var product float64 = 1
		for _, part := range parts {
			val, err := evaluateExpression(part)
			if err != nil {
				return 0, err
			}
			product *= val
		}
		return product, nil
	}

	if strings.Contains(expr, "/") {
		parts := strings.Split(expr, "/")
		if len(parts) == 0 {
			return 0, fmt.Errorf("无效表达式")
		}
		val, err := evaluateExpression(parts[0])
		if err != nil {
			return 0, err
		}
		for i := 1; i < len(parts); i++ {
			div, err := evaluateExpression(parts[i])
			if err != nil {
				return 0, err
			}
			if div == 0 {
				return 0, fmt.Errorf("除零错误")
			}
			val /= div
		}
		return val, nil
	}

	if strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") {
		return evaluateExpression(expr[1 : len(expr)-1])
	}

	return strconv.ParseFloat(expr, 64)
}

func init() {
	tool.MustRegisterTool(&CalculatorTool{})
}

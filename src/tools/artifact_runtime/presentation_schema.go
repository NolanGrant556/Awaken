package artifactruntime

import "awaken/src/tool"

func presentationCreateInputSchema() *tool.ToolParametersSchema {
	schema := tool.ToolParametersSchema{
		Type:                 "object",
		AdditionalProperties: tool.BoolPtr(false),
		Properties: map[string]tool.ToolParam{
			"output_path":    {Type: "string", Description: "输出 pptx 路径", MinLength: tool.IntPtr(1)},
			"title":          {Type: "string", Description: "演示文稿标题；无 slides 时用于默认标题页，有 slides 时写入 PPTX 文档标题元数据"},
			"theme":          presentationThemeSchema(),
			"slides":         {Type: "array", Description: "幻灯片数组", Items: presentationSlideSchema()},
			"render_preview": {Type: "boolean", Description: "是否保留 PDF/PNG 预览文件；false 仍执行技术验证"},
		},
		Required: []string{"output_path"},
	}
	return &schema
}

func presentationEditInputSchema() *tool.ToolParametersSchema {
	schema := tool.ToolParametersSchema{
		Type:                 "object",
		AdditionalProperties: tool.BoolPtr(false),
		Properties: map[string]tool.ToolParam{
			"source_path": {Type: "string", Description: "源 pptx 路径", MinLength: tool.IntPtr(1)},
			"output_path": {Type: "string", Description: "输出 pptx 路径，默认覆盖 source_path", MinLength: tool.IntPtr(1)},
			"theme":       presentationThemeSchema(),
			"edits": {
				Type:        "array",
				Description: "按请求顺序执行的增量编辑数组；每个 slide_index 都基于前一项执行后的当前页序",
				MinItems:    tool.IntPtr(1),
				Items:       presentationEditItemSchema(),
			},
			"render_preview": {Type: "boolean", Description: "是否保留 PDF/PNG 预览文件；false 仍执行技术验证"},
		},
		Required: []string{"source_path", "edits"},
	}
	return &schema
}

func presentationRenderInputSchema() *tool.ToolParametersSchema {
	schema := tool.ToolParametersSchema{
		Type:                 "object",
		AdditionalProperties: tool.BoolPtr(false),
		Properties: map[string]tool.ToolParam{
			"path":       {Type: "string", Description: "输入 pptx 路径", MinLength: tool.IntPtr(1)},
			"output_dir": {Type: "string", Description: "预览输出目录"},
		},
		Required: []string{"path"},
	}
	return &schema
}

func presentationThemeSchema() tool.ToolParam {
	return tool.ToolParam{
		Type:                 "object",
		AdditionalProperties: tool.BoolPtr(false),
		Properties: map[string]tool.ToolParam{
			"background": {Type: "string"},
			"surface":    {Type: "string"},
			"primary":    {Type: "string"},
			"accent":     {Type: "string"},
			"text":       {Type: "string"},
			"muted":      {Type: "string"},
			"font_latin": {Type: "string"},
			"font_ea":    {Type: "string"},
		},
	}
}

func presentationSlideSchema() *tool.ToolParam {
	return &tool.ToolParam{
		Type:                 "object",
		AdditionalProperties: tool.BoolPtr(false),
		Properties: map[string]tool.ToolParam{
			"title":      {Type: "string", Description: "页面主标题；所有 layout 都会渲染"},
			"subtitle":   {Type: "string", Description: "页面副标题；所有 layout 都会渲染"},
			"body":       {Type: "string", Description: "页面级导语或摘要；所有 layout 都会渲染，不会被 columns/metrics/timeline 吞掉"},
			"notes":      {Type: "string"},
			"background": {Type: "string"},
			"accent":     {Type: "string"},
			"layout": {
				Type: "string",
				Enum: []any{
					"title", "section", "bullets", "two_column", "image_right",
					"comparison", "metrics", "timeline", "closing",
				},
			},
			"bullets": {Type: "array", Description: "项目列表；用于 bullets/image_right/closing，或作为 two_column/comparison/metrics/timeline 的结构化内容回退；与对应结构化数组同时提供会被拒绝", Items: &tool.ToolParam{Type: "string"}},
			"images": {
				Type:  "array",
				Items: presentationImageSchema(),
			},
			"columns": {
				Type:        "array",
				Description: "two_column 最多 2 列，comparison 最多 3 列；其他 layout 不支持",
				MaxItems:    tool.IntPtr(3),
				Items: &tool.ToolParam{
					Type:                 "object",
					AdditionalProperties: tool.BoolPtr(false),
					Properties: map[string]tool.ToolParam{
						"title":   {Type: "string"},
						"bullets": {Type: "array", Items: &tool.ToolParam{Type: "string"}},
					},
				},
			},
			"metrics": {
				Type:        "array",
				Description: "metrics layout 的指标卡，最多 4 个；其他 layout 不支持",
				MaxItems:    tool.IntPtr(4),
				Items: &tool.ToolParam{
					Type:                 "object",
					AdditionalProperties: tool.BoolPtr(false),
					Properties: map[string]tool.ToolParam{
						"label": {Type: "string"},
						"value": {Type: "string"},
					},
				},
			},
			"timeline": {
				Type:        "array",
				Description: "timeline layout 的步骤，最多 5 个；detail 为短标签，body 为详细说明，两者都会渲染",
				MaxItems:    tool.IntPtr(5),
				Items: &tool.ToolParam{
					Type:                 "object",
					AdditionalProperties: tool.BoolPtr(false),
					Properties: map[string]tool.ToolParam{
						"title":  {Type: "string"},
						"detail": {Type: "string"},
						"body":   {Type: "string"},
					},
				},
			},
		},
	}
}

func presentationImageSchema() *tool.ToolParam {
	return &tool.ToolParam{
		Type:                 "object",
		AdditionalProperties: tool.BoolPtr(false),
		Properties: map[string]tool.ToolParam{
			"path":    {Type: "string", MinLength: tool.IntPtr(1)},
			"alt":     {Type: "string", Description: "写入 PPTX 图片替代文本元数据"},
			"caption": {Type: "string"},
			"fit":     {Type: "string", Enum: []any{"contain", "cover", "stretch"}},
			"x":       {Type: "number", Minimum: tool.FloatPtr(0)},
			"y":       {Type: "number", Minimum: tool.FloatPtr(0)},
			"width":   {Type: "number", Minimum: tool.FloatPtr(0)},
			"height":  {Type: "number", Minimum: tool.FloatPtr(0)},
		},
		Required: []string{"path"},
	}
}

func presentationEditItemSchema() *tool.ToolParam {
	return &tool.ToolParam{OneOf: []tool.ToolParam{
		presentationUpdateEditSchema(),
		presentationReplaceEditSchema(),
		presentationDeleteEditSchema(),
		presentationInsertEditSchema(),
	}}
}

func presentationUpdateEditSchema() tool.ToolParam {
	properties := presentationUpdateProperties()
	properties["op"] = tool.ToolParam{Type: "string", Enum: []any{"update"}}
	properties["slide_index"] = tool.ToolParam{Type: "integer", Minimum: tool.FloatPtr(0)}
	return tool.ToolParam{
		Type:                 "object",
		Description:          "增量更新；op 可省略并默认 update，至少提供一个可编辑字段",
		AdditionalProperties: tool.BoolPtr(false),
		Properties:           properties,
		Required:             []string{"slide_index"},
	}
}

func presentationReplaceEditSchema() tool.ToolParam {
	return tool.ToolParam{
		Type:                 "object",
		Description:          "整页替换；slide 必须提供完整替换页",
		AdditionalProperties: tool.BoolPtr(false),
		Properties: map[string]tool.ToolParam{
			"op":          {Type: "string", Enum: []any{"replace"}},
			"slide_index": {Type: "integer", Minimum: tool.FloatPtr(0)},
			"slide":       *presentationSlideSchema(),
		},
		Required: []string{"op", "slide_index", "slide"},
	}
}

func presentationDeleteEditSchema() tool.ToolParam {
	return tool.ToolParam{
		Type:                 "object",
		Description:          "删除当前页序中的一页",
		AdditionalProperties: tool.BoolPtr(false),
		Properties: map[string]tool.ToolParam{
			"op":          {Type: "string", Enum: []any{"delete"}},
			"slide_index": {Type: "integer", Minimum: tool.FloatPtr(0)},
		},
		Required: []string{"op", "slide_index"},
	}
}

func presentationInsertEditSchema() tool.ToolParam {
	return tool.ToolParam{
		Type:                 "object",
		Description:          "在当前页序插入完整 slide",
		AdditionalProperties: tool.BoolPtr(false),
		Properties: map[string]tool.ToolParam{
			"op":          {Type: "string", Enum: []any{"insert"}},
			"slide_index": {Type: "integer", Minimum: tool.FloatPtr(0)},
			"slide":       *presentationSlideSchema(),
		},
		Required: []string{"op", "slide_index", "slide"},
	}
}

func presentationUpdateProperties() map[string]tool.ToolParam {
	slide := presentationSlideSchema()
	return map[string]tool.ToolParam{
		"title":      slide.Properties["title"],
		"subtitle":   slide.Properties["subtitle"],
		"body":       slide.Properties["body"],
		"notes":      slide.Properties["notes"],
		"layout":     slide.Properties["layout"],
		"bullets":    slide.Properties["bullets"],
		"images":     slide.Properties["images"],
		"columns":    slide.Properties["columns"],
		"metrics":    slide.Properties["metrics"],
		"timeline":   slide.Properties["timeline"],
		"background": slide.Properties["background"],
		"accent":     slide.Properties["accent"],
		"slide":      *slide,
	}
}

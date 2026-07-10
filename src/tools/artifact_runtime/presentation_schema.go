package artifactruntime

import "awaken/src/tool"

func presentationCreateInputSchema() *tool.ToolParametersSchema {
	schema := tool.ToolParametersSchema{
		Type: "object",
		Properties: map[string]tool.ToolParam{
			"output_path":    {Type: "string", Description: "输出 pptx 路径"},
			"title":          {Type: "string", Description: "演示文稿标题"},
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
		Type: "object",
		Properties: map[string]tool.ToolParam{
			"source_path": {Type: "string", Description: "源 pptx 路径"},
			"output_path": {Type: "string", Description: "输出 pptx 路径，默认覆盖 source_path"},
			"theme":       presentationThemeSchema(),
			"edits": {
				Type:        "array",
				Description: "增量编辑数组",
				MinItems:    tool.IntPtr(1),
				Items:       presentationEditItemSchema(),
			},
			"render_preview": {Type: "boolean", Description: "是否保留 PDF/PNG 预览文件；false 仍执行技术验证"},
		},
		Required: []string{"source_path", "edits"},
	}
	return &schema
}

func presentationThemeSchema() tool.ToolParam {
	return tool.ToolParam{
		Type: "object",
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
		Type: "object",
		Properties: map[string]tool.ToolParam{
			"title":      {Type: "string"},
			"subtitle":   {Type: "string"},
			"body":       {Type: "string"},
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
			"bullets": {Type: "array", Items: &tool.ToolParam{Type: "string"}},
			"images": {
				Type:  "array",
				Items: presentationImageSchema(),
			},
			"columns": {
				Type: "array",
				Items: &tool.ToolParam{
					Type: "object",
					Properties: map[string]tool.ToolParam{
						"title":   {Type: "string"},
						"bullets": {Type: "array", Items: &tool.ToolParam{Type: "string"}},
					},
				},
			},
			"metrics": {
				Type: "array",
				Items: &tool.ToolParam{
					Type: "object",
					Properties: map[string]tool.ToolParam{
						"label": {Type: "string"},
						"value": {Type: "string"},
					},
				},
			},
			"timeline": {
				Type: "array",
				Items: &tool.ToolParam{
					Type: "object",
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
		Type: "object",
		Properties: map[string]tool.ToolParam{
			"path":    {Type: "string"},
			"alt":     {Type: "string"},
			"caption": {Type: "string"},
			"fit":     {Type: "string", Enum: []any{"contain", "cover", "stretch"}},
			"x":       {Type: "number"},
			"y":       {Type: "number"},
			"width":   {Type: "number"},
			"height":  {Type: "number"},
		},
	}
}

func presentationEditItemSchema() *tool.ToolParam {
	return &tool.ToolParam{
		Type: "object",
		Properties: map[string]tool.ToolParam{
			"op": {
				Type: "string",
				Enum: []any{"update", "replace", "delete", "insert"},
			},
			"slide_index": {Type: "integer", Minimum: tool.FloatPtr(0)},
			"title":       {Type: "string"},
			"subtitle":    {Type: "string"},
			"body":        {Type: "string"},
			"notes":       {Type: "string"},
			"layout": {
				Type: "string",
				Enum: []any{
					"title", "section", "bullets", "two_column", "image_right",
					"comparison", "metrics", "timeline", "closing",
				},
			},
			"bullets": {Type: "array", Items: &tool.ToolParam{Type: "string"}},
			"images":  {Type: "array", Items: presentationImageSchema()},
			"slide":   *presentationSlideSchema(),
		},
		Required: []string{"slide_index"},
	}
}

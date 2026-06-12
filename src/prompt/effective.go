package prompt

func BuildEffectiveSystemPrompt(staticContent string, runtimeInfo RuntimeInfo) string {
	sp := NewSystemPrompt(staticContent)
	sp.SetRuntimeInfo(runtimeInfo)
	return sp.Build()
}

func BuildEffectiveSystemPromptWithDefaults() (string, error) {
	staticContent, err := LoadDefaultPrompt()
	if err != nil {
		return "", err
	}
	runtimeInfo := NewRuntimeInfo()
	return BuildEffectiveSystemPrompt(staticContent, runtimeInfo), nil
}

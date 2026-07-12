//go:build !darwin

package tool

func cloneWorkspaceRegularFile(source string, destination string) (bool, error) {
	return false, nil
}

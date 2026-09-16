package managedenv

import (
	"errors"
	"os"
	"runtime"
	"syscall"
)

func classifyPreparationError(err error) error {
	if err == nil {
		return nil
	}
	var typed *Error
	if errors.As(err, &typed) {
		return err
	}
	if errors.Is(err, syscall.ENOSPC) || (runtime.GOOS == "windows" && (errors.Is(err, syscall.Errno(112)) || errors.Is(err, syscall.Errno(39)))) {
		return failure("environment_disk_full", "共享依赖目录所在磁盘空间不足；释放空间后可重试，已有健康环境保留")
	}
	if os.IsPermission(err) {
		return failure("environment_permission_denied", "共享依赖目录不可读写；恢复当前用户的目录访问权限后可重试")
	}
	return err
}

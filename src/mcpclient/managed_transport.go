package mcpclient

import (
	"context"
	"fmt"
	"os/exec"
	"sync"

	"awaken/src/managedenv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type managedCommandTransport struct {
	command *exec.Cmd
	manager *managedenv.Manager
}

func (t *managedCommandTransport) Connect(ctx context.Context) (mcp.Connection, error) {
	launch, env, release, err := t.manager.PrepareCommand(ctx, t.command.Path, t.command.Args[1:], t.command.Env, "mcp")
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(launch.Executable, launch.Args...)
	cmd.Dir = t.command.Dir
	cmd.Env = env
	connection, err := (&mcp.CommandTransport{Command: cmd}).Connect(ctx)
	if err != nil {
		release()
		if _, recognized := managedenv.RecognizePackageCommand(t.command.Path, t.command.Args[1:]); !recognized {
			return nil, fmt.Errorf("自定义 MCP 命令按外部配置启动失败；请核对可执行文件和配置路径，共享管理器不会猜测安装包: %w", err)
		}
		return nil, err
	}
	if launch.ManagedInstanceID != "" && cmd.Process != nil {
		hold, err := t.manager.HoldProcess(managedenv.Binding{InstanceID: launch.ManagedInstanceID}, cmd.Process.Pid)
		if err != nil {
			_ = connection.Close()
			release()
			return nil, err
		}
		originalRelease := release
		release = func() { hold(); originalRelease() }
	}
	return &managedConnection{Connection: connection, release: release}, nil
}

type managedConnection struct {
	mcp.Connection
	once    sync.Once
	release func()
}

func (c *managedConnection) Close() error {
	err := c.Connection.Close()
	c.once.Do(c.release)
	return err
}

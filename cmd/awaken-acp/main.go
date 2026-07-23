package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"awaken/src/acp/agentbridge"
)

func main() {
	startup := flag.String("startup", "desktop", "Startup strategy when no ready Core exists: desktop, core, fail")
	userDataDir := flag.String("user-data-dir", "", "Explicit Awaken userData directory (dev/tests)")
	identity := flag.String("identity", "", "Explicit runtime identity override (dev/tests)")
	coreBinary := flag.String("core-binary", "", "Path to awaken-core binary (default: sibling of awaken-acp)")
	flag.Parse()

	if *userDataDir == "" {
		var err error
		*userDataDir, err = agentbridge.DefaultUserDataDir(false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "awaken-acp: resolve default userData dir: %v\n", err)
			os.Exit(1)
		}
	}

	strategy := agentbridge.StartupStrategy(*startup)
	if strategy != agentbridge.StartupDesktop && strategy != agentbridge.StartupCore && strategy != agentbridge.StartupFail {
		fmt.Fprintf(os.Stderr, "awaken-acp: unknown startup strategy %q\n", *startup)
		os.Exit(1)
	}

	coreBinaryPath := *coreBinary
	if coreBinaryPath == "" {
		var err error
		coreBinaryPath, err = defaultCoreBinaryPath()
		if err != nil {
			fmt.Fprintf(os.Stderr, "awaken-acp: resolve default core binary: %v\n", err)
			os.Exit(1)
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	launcher := agentbridge.Launcher{
		UserDataDir:    *userDataDir,
		Strategy:       strategy,
		IdentityParam:  *identity,
		CoreBinaryPath: coreBinaryPath,
	}

	res, err := launcher.Launch(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "awaken-acp: launch failed: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := res.Close(); err != nil {
			log.Printf("awaken-acp: release lease error: %v", err)
		}
	}()

	log.Printf("awaken-acp: attached to %s (owner=%s lease=%s)", res.Identity.InstanceID, res.OwnerMode, res.LeaseID)

	coreClient := agentbridge.NewCoreClient(res.BaseURL, res.ControlToken)
	if err := coreClient.VerifyIdentity(ctx, res.Identity); err != nil {
		fmt.Fprintf(os.Stderr, "awaken-acp: identity verification failed: %v\n", err)
		os.Exit(1)
	}

	agent := agentbridge.NewAgent(coreClient)
	server := agentbridge.Server{
		Agent:      agent,
		LeaseID:    res.LeaseID,
		CoreClient: coreClient,
	}
	if err := server.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(os.Stderr, "awaken-acp: server error: %v\n", err)
		os.Exit(1)
	}
	log.Println("awaken-acp: shutting down")
}

func defaultCoreBinaryPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		if len(os.Args) == 0 {
			return "", err
		}
		exe = os.Args[0]
	}
	name := "awaken-core"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(filepath.Dir(exe), name), nil
}

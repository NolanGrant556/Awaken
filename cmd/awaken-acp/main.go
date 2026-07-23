package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"awaken/src/acp/agentbridge"
)

func main() {
	startup := flag.String("startup", "desktop", "Startup strategy when no ready Core exists: desktop, core, fail")
	userDataDir := flag.String("user-data-dir", "", "Explicit Awaken userData directory (dev/tests)")
	identity := flag.String("identity", "", "Explicit runtime identity override (dev/tests)")
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

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	launcher := agentbridge.Launcher{
		UserDataDir:   *userDataDir,
		Strategy:      strategy,
		IdentityParam: *identity,
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
	// TODO(Stage 5): instantiate stdio ACP server using res.BaseURL and res.ControlToken.

	<-ctx.Done()
	log.Println("awaken-acp: shutting down")
}

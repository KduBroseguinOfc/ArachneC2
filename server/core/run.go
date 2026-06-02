package core

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/portbuster1337/arachne-c2/pkg/cryptography"
)

func Run(relayAddrs []string) error {
	keys, err := cryptography.LoadOrGenerateOperatorKey(keyPath())
	if err != nil {
		return fmt.Errorf("load operator key: %w", err)
	}

	log.Printf("[operator] peer ID: %s", keys.PeerID.String())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	op, err := NewOperator(ctx, keys, relayAddrs)
	if err != nil {
		return fmt.Errorf("create operator: %w", err)
	}
	defer op.Close()

	if err := op.Start(); err != nil {
		return fmt.Errorf("start operator: %w", err)
	}

	op.RunCLI()
	return nil
}

func keyPath() string {
	exe, err := os.Executable()
	if err == nil {
		return filepath.Join(filepath.Dir(exe), "operator.key")
	}
	return "operator.key"
}

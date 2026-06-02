package core

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/anomalyco/arachne-c2/pkg/cryptography"
)

func Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	keyPath := "operator.key"
	keys, err := cryptography.LoadOrGenerateOperatorKey(keyPath)
	if err != nil {
		return fmt.Errorf("load operator key: %w", err)
	}

	log.Printf("[arachne] operator peer ID: %s", keys.PeerID.String())
	log.Printf("[arachne] key saved to: %s", keyPath)

	op, err := NewOperator(ctx, keys)
	if err != nil {
		return fmt.Errorf("create operator: %w", err)
	}

	if err := op.Start(); err != nil {
		return fmt.Errorf("start operator: %w", err)
	}

	<-sigCh
	log.Println("[arachne] shutting down...")
	op.Close()
	return nil
}

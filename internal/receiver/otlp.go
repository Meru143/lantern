// Package receiver starts and stops the embedded OTLP receiver.
package receiver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/Meru143/lantern/internal/config"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	collectorreceiver "go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
)

var (
	runtimeMu         sync.Mutex
	startedComponents []component.Component
)

// Start starts the embedded OTLP gRPC and HTTP receivers.
func Start(ctx context.Context, cfg *config.Config, signalConsumer *SignalConsumer) error {
	grpcEndpoint := fmt.Sprintf("0.0.0.0:%d", cfg.OTLPGRPCPort)
	httpEndpoint := fmt.Sprintf("0.0.0.0:%d", cfg.OTLPHTTPPort)

	if err := preflightListen(grpcEndpoint, cfg.OTLPGRPCPort, "--otlp-grpc"); err != nil {
		return err
	}
	if err := preflightListen(httpEndpoint, cfg.OTLPHTTPPort, "--otlp-http"); err != nil {
		return err
	}

	factory := otlpreceiver.NewFactory()
	receiverConfig := factory.CreateDefaultConfig().(*otlpreceiver.Config)
	receiverConfig.GRPC.NetAddr.Endpoint = grpcEndpoint
	receiverConfig.HTTP.Endpoint = httpEndpoint

	settings := collectorreceiver.Settings{
		ID: component.MustNewID("otlp"),
	}

	traceReceiver, err := factory.CreateTraces(ctx, settings, receiverConfig, signalConsumer)
	if err != nil {
		return err
	}
	logReceiver, err := factory.CreateLogs(ctx, settings, receiverConfig, signalConsumer)
	if err != nil {
		return err
	}

	components := uniqueComponents(traceReceiver, logReceiver)
	host := componenttest.NewNopHost()
	started := make([]component.Component, 0, len(components))
	for _, comp := range components {
		if err := comp.Start(ctx, host); err != nil {
			_ = stopComponents(ctx, started)
			return err
		}
		started = append(started, comp)
	}

	runtimeMu.Lock()
	startedComponents = started
	runtimeMu.Unlock()

	return nil
}

// Shutdown stops any OTLP receiver components started by Start.
func Shutdown(ctx context.Context) error {
	runtimeMu.Lock()
	components := startedComponents
	startedComponents = nil
	runtimeMu.Unlock()

	return stopComponents(ctx, components)
}

func stopComponents(ctx context.Context, components []component.Component) error {
	var errs []error
	for i := len(components) - 1; i >= 0; i-- {
		if err := components[i].Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func uniqueComponents(components ...component.Component) []component.Component {
	unique := make([]component.Component, 0, len(components))
	for _, candidate := range components {
		seen := false
		for _, existing := range unique {
			if existing == candidate {
				seen = true
				break
			}
		}
		if !seen {
			unique = append(unique, candidate)
		}
	}
	return unique
}

func preflightListen(endpoint string, port int, flagName string) error {
	listener, err := net.Listen("tcp", endpoint)
	if err != nil {
		if isAddressInUse(err) {
			return fmt.Errorf("LT001: Port %d is already in use. Use %s to specify a different port.", port, flagName)
		}
		return err
	}
	return listener.Close()
}

func isAddressInUse(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "address already in use") || strings.Contains(message, "only one usage of each socket address")
}

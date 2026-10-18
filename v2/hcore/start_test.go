package hcore

import (
	"context"
	"fmt"
	"testing"

	"github.com/sagernet/sing-box/common/monitoring"
	"github.com/sagernet/sing-box/daemon"
	"github.com/stretchr/testify/require"
)

func TestBuildConfig_StartServicePanic(t *testing.T) {
	ctx := context.Background()

	in := &StartRequest{
		ConfigContent: "{}",
	}

	_, err := StartService(ctx, in)
	require.Error(t, err)
	require.Equal(t, static.CoreState, CoreStates_STOPPED)
}

func TestStartService_CancelledRequestContext(t *testing.T) {
	request, cancel := context.WithCancel(context.Background())
	cancel()
	ctx := withRequestContext(context.Background(), request)

	response, err := StartService(ctx, &StartRequest{ConfigContent: "{}"})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, CoreStates_STOPPED, response.CoreState)
	require.Equal(t, CoreStates_STOPPED, static.CoreState)
	require.Nil(t, static.StartedService)

	response, err = Restart(ctx, &StartRequest{ConfigContent: "{}"})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, CoreStates_STOPPED, static.CoreState)

	_, err = HotReload(ctx, &StartRequest{ConfigContent: "{}"})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, CoreStates_STOPPED, static.CoreState)
}

func TestStartService_PreCancelledPreservesLifecycle(t *testing.T) {
	testCancelledStartPreservesLifecycle(t, false)
}

func TestStartService_CancelledWhileLockHeldPreservesLifecycle(t *testing.T) {
	testCancelledStartPreservesLifecycle(t, true)
}

func testCancelledStartPreservesLifecycle(t *testing.T, cancelWhileLocked bool) {
	t.Helper()
	for _, state := range []CoreStates{
		CoreStates_STOPPED, CoreStates_STARTING, CoreStates_STARTED,
		CoreStates_STOPPING, CoreStates_HOT_RELOADING,
	} {
		for _, hasService := range []bool{false, true} {
			for _, nilRequest := range []bool{false, true} {
				for _, attachedRequest := range []bool{false, true} {
					name := fmt.Sprintf("%s/service=%t/nil-request=%t/attached-request=%t", state, hasService, nilRequest, attachedRequest)
					t.Run(name, func(t *testing.T) {
						original := static
						observerContext, stopObservers := context.WithCancel(context.Background())
						instance := &HiddifyInstance{
							CoreState:            state,
							BaseContext:          context.Background(),
							previousStartRequest: &StartRequest{ConfigName: "existing request"},
							coreInfoObserver:     monitoring.NewBroadcaster[*CoreInfoResponse](observerContext),
							logObserver:          monitoring.NewBroadcaster[*LogMessage](observerContext),
						}
						// A zero-value service is an identity sentinel only. Never start or close it.
						var runningService *daemon.StartedService
						if hasService {
							runningService = &daemon.StartedService{}
						}
						instance.StartedService = runningService
						previousRequest := instance.previousStartRequest
						static = instance
						t.Cleanup(func() {
							static = original
							stopObservers()
						})

						request, cancel := context.WithCancel(context.Background())
						defer cancel()
						var ctx context.Context = request
						if attachedRequest {
							ctx = withRequestContext(instance.BaseContext, request)
						}
						var in *StartRequest
						if !nilRequest {
							in = &StartRequest{ConfigContent: "{}"}
						}

						var response *CoreInfoResponse
						var err error
						if cancelWhileLocked {
							type result struct {
								response *CoreInfoResponse
								err      error
							}
							entered := make(chan struct{})
							finished := make(chan result, 1)
							instance.lock.Lock()
							// The lock holder finishes a preceding lifecycle transition before the
							// cancelled request may inspect state. Channels replace timing sleeps.
							instance.CoreState = CoreStates_STARTING
							instance.StartedService = nil
							go func() {
								close(entered)
								response, err := StartService(ctx, in)
								finished <- result{response: response, err: err}
							}()
							<-entered
							cancel()
							instance.CoreState = state
							instance.StartedService = runningService
							instance.lock.Unlock()
							completed := <-finished
							response, err = completed.response, completed.err
						} else {
							cancel()
							response, err = StartService(ctx, in)
						}

						// Cancellation must return before loading a nil request from a database,
						// building config, or using the sentinel as a real network service.
						require.ErrorIs(t, err, context.Canceled)
						require.NotNil(t, response)
						require.Equal(t, state, response.CoreState)
						require.Equal(t, MessageType_INSTANCE_NOT_STARTED, response.MessageType)
						require.Equal(t, "start cancelled", response.Message)
						require.Equal(t, state, instance.CoreState)
						if runningService == nil {
							require.Nil(t, instance.StartedService)
						} else {
							require.Same(t, runningService, instance.StartedService)
						}
						require.Same(t, previousRequest, instance.previousStartRequest)
					})
				}
			}
		}
	}
}

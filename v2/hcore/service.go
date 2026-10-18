package hcore

import (
	"context"

	box "github.com/sagernet/sing-box"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/urltest"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/daemon"
	"github.com/sagernet/sing-box/experimental/clashapi"
	"github.com/sagernet/sing-box/experimental/clashapi/trafficontrol"
	"github.com/sagernet/sing-box/experimental/libbox"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json"
	"github.com/sagernet/sing/service"
)

func NewService(ctx context.Context, options option.Options) (*daemon.StartedService, error) {

	// ctx = filemanager.WithDefault(ctx, sWorkingPath, sTempPath, sUserID, sGroupID)
	bopts := newServiceOptions(ctx, C.IsIos)
	err := libbox.CheckConfigOptions(&options)
	if err != nil {
		return nil, err
	}
	instance := daemon.NewStartedService(bopts)

	// for i := 0; i < 10; i++ {
	// 	if hutils.IsPortInUse(options.Inbounds[0].SocksOptions.ListenPort) {
	// 		<-time.After(100 * time.Millisecond)
	// 	}
	// }

	content, err := json.MarshalContext(ctx, options)
	if err != nil {
		return nil, err
	}
	if err := instance.StartOrReloadService(ctx, string(content), nil); err != nil {
		return nil, err
	}

	// instance.GetInstance().AddPostService("hiddifyMainServiceManager", &hiddifyMainServiceManager{})

	// if err := startCommandServer(instance); err != nil {
	// 	return errorWrapper(MessageType_START_COMMAND_SERVER, err)
	// }

	return instance, nil
}

func newServiceOptions(ctx context.Context, isIOS bool) daemon.ServiceOptions {
	return daemon.ServiceOptions{
		Context:          ctx,
		Debug:            static.debug,
		LogMaxLines:      100,
		OOMKillerEnabled: isIOS,
		Handler:          &LogInterface{},
		ExtraServices: []adapter.LifecycleService{
			&hiddifyMainServiceManager{},
		},
	}
}

func (h *HiddifyInstance) UrlTestHistory() *urltest.HistoryStorage {

	ins := h.Instance()
	if ins == nil {
		return nil
	}
	return ins.UrlTestHistory()
}

func (h *HiddifyInstance) Box() *box.Box {
	ins := h.Instance()
	if ins == nil {
		return nil
	}
	return ins.Box()
}

func (h *HiddifyInstance) Instance() *daemon.Instance {
	ss := h.StartedService
	if ss == nil {
		return nil
	}
	return ss.Instance()

}

func (h *HiddifyInstance) Context() context.Context {
	ins := h.Instance()
	if ins == nil {
		return nil
	}
	return ins.Context()
}

func (h *HiddifyInstance) TrafficManager() *trafficontrol.Manager {
	if ctx := h.Context(); ctx != nil {
		if s, ok := service.FromContext[adapter.ClashServer](ctx).(*clashapi.Server); ok {
			return s.TrafficManager()
		}
	}
	return nil
}

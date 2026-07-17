package api

import (
	"os"
	"path/filepath"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/notify"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/venv"
	"be/internal/ws"
)

// buildNotifySubsystem wires the notification subsystem: the dispatcher
// (registered as a hub listener — must run before hub.Run()), the async
// delivery worker, and the script transport runtime.
func buildNotifySubsystem(pool *db.Pool, clk clock.Clock, hub *ws.Hub, errorSvc *service.ErrorService, dataPath, sdkDir string) (service.NotificationWaker, *notify.Worker) {
	notifyWakeCh := make(chan struct{}, 8)
	channelRepo := repo.NewNotificationChannelRepo(pool, clk)
	deliveryRepo := repo.NewNotificationDeliveryRepo(pool, clk)
	projectRepoForNotify := repo.NewProjectRepo(pool, clk)
	ticketRepoForNotify := repo.NewTicketRepo(pool, clk)
	dispatcher := notify.NewDispatcher(
		channelRepo,
		deliveryRepo,
		notify.ProjectLookupFunc(func(id string) (string, bool, error) {
			p, err := projectRepoForNotify.Get(id)
			if err != nil {
				return "", false, err
			}
			return p.Name, true, nil
		}),
		notify.TicketLookupFunc(func(pid, tid string) (string, bool, error) {
			t, err := ticketRepoForNotify.Get(pid, tid)
			if err != nil {
				return "", false, err
			}
			return t.Title, true, nil
		}),
		notifyWakeCh,
	)
	hub.RegisterListener(dispatcher)
	notifyWorker := notify.NewWorker(deliveryRepo, channelRepo, hub, errorSvc, clk, notifyWakeCh)

	// Script notification transport runtime.
	dataDir := ""
	if dataPath != "" {
		dataDir = filepath.Dir(dataPath)
	}
	notify.RegisterScriptRuntime(&notify.ScriptRuntime{
		ProjectRepo: repo.NewProjectRepo(pool, clk),
		VenvMgr:     venv.New(dataDir, clk),
		EnvVarRepo:  repo.NewProjectEnvVarRepo(pool, clk),
		SessionRepo: repo.NewAgentSessionRepo(pool, clk),
		Clock:       clk,
		SDKDir:      sdkDir,
		SocketPath:  os.Getenv("NRFLO_SOCKET"),
		NrfloHome:   dataDir,
	})

	return service.NewChanWaker(notifyWakeCh), notifyWorker
}

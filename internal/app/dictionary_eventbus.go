package app

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lihongjie0209/dictionary-service/internal/config"
	"github.com/lihongjie0209/microservice-platform-go/eventbus"
	platformoutbox "github.com/lihongjie0209/microservice-platform-go/outbox"
	"go.uber.org/fx"
)

type dictionaryEventRuntime struct {
	config config.Config
	store  *platformoutbox.SQLStore
	logger *slog.Logger
	cancel context.CancelFunc
	wg     sync.WaitGroup
	bus    *eventbus.Bus
}

func newDictionaryEventRuntime(lc fx.Lifecycle, cfg config.Config, store *platformoutbox.SQLStore, logger *slog.Logger) *dictionaryEventRuntime {
	runtime := &dictionaryEventRuntime{config: cfg, store: store, logger: logger}
	lc.Append(fx.Hook{OnStart: runtime.start, OnStop: runtime.stop})
	return runtime
}
func (r *dictionaryEventRuntime) start(ctx context.Context) error {
	if !r.config.EventBus.Enabled {
		return nil
	}
	if r.store == nil {
		return errors.New("enabled event bus requires dictionary outbox")
	}
	bus, err := eventbus.New(ctx, eventbus.Config{URLs: r.config.EventBus.URLs, ClientName: r.config.App.Name, StreamName: r.config.EventBus.StreamName, Subjects: []string{"platform.>"}, Storage: r.config.EventBus.Storage, MaxAge: r.config.EventBus.MaxAge, DuplicateWindow: r.config.EventBus.DuplicateWindow, ConnectTimeout: r.config.EventBus.ConnectTimeout, PublishTimeout: r.config.EventBus.PublishTimeout})
	if err != nil {
		return err
	}
	dispatcher, err := platformoutbox.New(r.store, bus, platformoutbox.Config{BatchSize: r.config.EventBus.DispatchBatchSize, Lease: r.config.EventBus.DispatchLease, RetryDelay: r.config.EventBus.DispatchRetryDelay})
	if err != nil {
		_ = bus.Close()
		return err
	}
	r.bus = bus
	runCtx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	cleaner, err := platformoutbox.NewRetentionCleaner(r.store, platformoutbox.RetentionConfig{Retention: r.config.EventBus.PublishedRetention, BatchSize: r.config.EventBus.CleanupBatchSize})
	if err != nil {
		cancel()
		_ = bus.Close()
		return err
	}
	r.wg.Go(func() {
		ticker := time.NewTicker(r.config.EventBus.DispatchInterval)
		defer ticker.Stop()
		for {
			if _, err := dispatcher.RunOnce(runCtx); err != nil && !errors.Is(err, context.Canceled) {
				r.logger.ErrorContext(runCtx, "dispatch dictionary outbox failed", "error", err)
			}
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
			}
		}
	})
	r.wg.Go(func() {
		ticker := time.NewTicker(r.config.EventBus.CleanupInterval)
		defer ticker.Stop()
		for {
			if deleted, runErr := cleaner.RunOnce(runCtx); runErr != nil && !errors.Is(runErr, context.Canceled) {
				r.logger.ErrorContext(runCtx, "clean published dictionary outbox events", "error", runErr)
			} else if deleted > 0 {
				r.logger.InfoContext(runCtx, "published dictionary outbox events cleaned", "deleted", deleted)
			}
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
			}
		}
	})
	return nil
}
func (r *dictionaryEventRuntime) stop(context.Context) error {
	if r.cancel != nil {
		r.cancel()
		r.wg.Wait()
	}
	if r.bus != nil {
		return r.bus.Close()
	}
	return nil
}
func newDictionaryOutboxStore(db *sqlx.DB) (*platformoutbox.SQLStore, error) {
	if db == nil {
		return nil, nil
	}
	return platformoutbox.NewSQLStore(db, "dictionary_outbox_events")
}

var DictionaryEventBusModule = fx.Module("dictionary-event-bus", fx.Provide(newDictionaryOutboxStore, newDictionaryEventRuntime), fx.Invoke(func(*dictionaryEventRuntime) {}))

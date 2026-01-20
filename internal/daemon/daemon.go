package daemon

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oxisoft/oxiwatch/internal/config"
	"github.com/oxisoft/oxiwatch/internal/geoip"
	"github.com/oxisoft/oxiwatch/internal/journal"
	"github.com/oxisoft/oxiwatch/internal/notifier"
	"github.com/oxisoft/oxiwatch/internal/parser"
	"github.com/oxisoft/oxiwatch/internal/report"
	"github.com/oxisoft/oxiwatch/internal/scheduler"
	"github.com/oxisoft/oxiwatch/internal/storage"
)

type Daemon struct {
	cfg       *config.Config
	logger    *slog.Logger
	storage   *storage.Storage
	journal   *journal.Reader
	telegram  *notifier.Telegram
	scheduler *scheduler.Scheduler
	geoip     *geoip.Resolver
	geoUpdate *geoip.Updater
	report    *report.Generator
}

func New(cfg *config.Config, logger *slog.Logger) (*Daemon, error) {
	store, err := storage.New(cfg.DatabasePath)
	if err != nil {
		return nil, err
	}

	d := &Daemon{
		cfg:       cfg,
		logger:    logger,
		storage:   store,
		journal:   journal.New(logger),
		telegram:  notifier.NewTelegram(cfg.TelegramBotToken, cfg.TelegramChatID, cfg.ServerName),
		scheduler: scheduler.New(logger),
		geoUpdate: geoip.NewUpdater(cfg.GeoIPDatabasePath, logger),
		report:    report.NewGenerator(store, cfg.ServerName),
	}

	if cfg.GeoIPEnabled {
		if err := d.initGeoIP(); err != nil {
			logger.Warn("GeoIP initialization failed, continuing without geo lookup", "error", err)
		}
	}

	return d, nil
}

func (d *Daemon) initGeoIP() error {
	if !d.geoUpdate.DatabaseExists() {
		d.logger.Info("GeoIP database not found, downloading...")
		if err := d.geoUpdate.Update(); err != nil {
			d.logger.Warn("failed to download GeoIP database", "error", err)
			return nil
		}
	}

	if d.geoUpdate.DatabaseExists() {
		resolver, err := geoip.NewResolver(d.cfg.GeoIPDatabasePath)
		if err != nil {
			return err
		}
		d.geoip = resolver
		d.logger.Info("GeoIP database loaded", "path", d.cfg.GeoIPDatabasePath)
	}

	return nil
}

func (d *Daemon) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	if err := d.journal.Start(ctx); err != nil {
		return err
	}
	d.logger.Info("started monitoring SSH journal")

	if d.cfg.DailyReportEnabled {
		if err := d.scheduler.AddDailyTask("daily-report", d.cfg.DailyReportTime, d.cfg.DailyReportTimezone, d.sendDailyReport); err != nil {
			return err
		}
		d.logger.Info("scheduled daily report", "time", d.cfg.DailyReportTime, "timezone", d.cfg.DailyReportTimezone)
	}

	if err := d.scheduler.AddDailyTask("retention-cleanup", "03:00", "UTC", d.runCleanup); err != nil {
		return err
	}

	if d.cfg.GeoIPEnabled {
		if err := d.scheduler.AddMonthlyTask("geoip-update", "04:00", "UTC", d.checkGeoIPUpdate); err != nil {
			return err
		}
	}

	go d.scheduler.Start(ctx)

	d.logger.Info("daemon started")

	for {
		select {
		case sig := <-sigCh:
			d.logger.Info("received signal, shutting down", "signal", sig)
			cancel()
			return d.shutdown()

		case event := <-d.journal.Events():
			if event == nil {
				d.logger.Info("journal reader closed")
				return d.shutdown()
			}
			d.processEvent(event)
		}
	}
}

func (d *Daemon) processEvent(event *parser.SSHEvent) {
	var country, city string
	if d.geoip != nil {
		loc, err := d.geoip.Lookup(event.IP)
		if err != nil {
			d.logger.Warn("GeoIP lookup failed", "ip", event.IP, "error", err)
		} else if loc != nil {
			country = loc.Country
			city = loc.City
		}
	}

	if err := d.storage.InsertEvent(event, country, city); err != nil {
		d.logger.Error("failed to store event", "error", err)
		return
	}

	if event.EventType == parser.EventSuccess {
		d.logger.Info("successful SSH login",
			"user", event.Username,
			"ip", event.IP,
			"method", event.Method,
			"country", country,
			"city", city,
		)

		if err := d.telegram.SendLoginAlert(event, country, city); err != nil {
			d.logger.Error("failed to send Telegram alert", "error", err)
		}
	} else {
		d.logger.Debug("failed SSH attempt",
			"user", event.Username,
			"ip", event.IP,
			"invalid_user", event.InvalidUser,
		)
	}
}

func (d *Daemon) sendDailyReport(ctx context.Context) error {
	yesterday := time.Now().AddDate(0, 0, -1)
	reportText, err := d.report.GenerateDailyReport(yesterday)
	if err != nil {
		return err
	}
	return d.telegram.SendDailyReport(reportText)
}

func (d *Daemon) runCleanup(ctx context.Context) error {
	deleted, err := d.storage.Cleanup(d.cfg.RetentionDays)
	if err != nil {
		return err
	}
	if deleted > 0 {
		d.logger.Info("retention cleanup completed", "deleted", deleted)
	}
	return nil
}

func (d *Daemon) checkGeoIPUpdate(ctx context.Context) error {
	needsUpdate, err := d.geoUpdate.NeedsUpdate()
	if err != nil {
		d.logger.Warn("failed to check for GeoIP update", "error", err)
		return nil
	}

	if needsUpdate {
		if err := d.geoUpdate.Update(); err != nil {
			return err
		}

		if d.geoip != nil {
			d.geoip.Close()
		}
		resolver, err := geoip.NewResolver(d.cfg.GeoIPDatabasePath)
		if err != nil {
			return err
		}
		d.geoip = resolver
	}
	return nil
}

func (d *Daemon) shutdown() error {
	d.logger.Info("shutting down")

	if d.journal != nil {
		d.journal.Stop()
	}

	if d.geoip != nil {
		d.geoip.Close()
	}

	if d.storage != nil {
		d.storage.Close()
	}

	return nil
}

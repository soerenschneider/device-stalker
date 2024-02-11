package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/soerenschneider/device-stalker/internal"
)

const (
	AppName = "device-stalker"
)

var (
	defaultConfigFile = fmt.Sprintf("/etc/%s.yaml", AppName)

	flagConfigFile string
	flagVersion    bool

	sendPayloadOnError = true

	BuildVersion string
	CommitHash   string
)

type Sampler interface {
	Check(ctx context.Context) (bool, error)
	Device() internal.Device
}

type Notifier interface {
	Notify(ctx context.Context, probe internal.Device, val string) error
}

type App struct {
	samplers map[string]Sampler
	notifier Notifier
}

func main() {
	parseFlags()

	log.Info().Msgf("Starting version %s", BuildVersion)
	conf, err := internal.ReadConfig(flagConfigFile)
	if err != nil {
		log.Fatal().Err(err).Msg("could not read config file")
	}

	if err := internal.ValidateConfig(conf); err != nil {
		log.Fatal().Err(err).Msg("could not validate config")
	}

	notifier, err := buildNotifier(conf)
	if err != nil {
		log.Fatal().Err(err).Msg("could not build notifier")
	}

	samplers, err := buildSamplers(conf)
	if err != nil {
		log.Fatal().Err(err).Msg("could not build samplers")
	}

	go func() {
		internal.VersionMetric.WithLabelValues(BuildVersion, CommitHash).Set(1)
		internal.ProcessStartTime.SetToCurrentTime()
		if err := internal.StartMetricsServer(conf.MetricsAddr); err != nil {
			log.Fatal().Err(err).Msg("can not start metrics server")
		}
	}()

	app := &App{
		samplers: samplers,
		notifier: notifier,
	}

	app.run()
}

func (app *App) run() {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT)
		<-sig
		log.Info().Msg("Got signal, quitting")
		cancel()
	}()

	ticker := time.NewTicker(30 * time.Second)
	app.tick(ctx)
	for {
		select {
		case <-ticker.C:
			app.tick(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func translateOutcome(outcome bool) string {
	if outcome {
		return "ON"
	}
	return "OFF"
}

func (app *App) tick(ctx context.Context) {
	wg := &sync.WaitGroup{}
	for name, probe := range app.samplers {
		wg.Add(1)
		go func(name string, prober Sampler, w *sync.WaitGroup) {
			isPresent, err := prober.Check(ctx)
			log.Info().Msgf("Probed %v=%t", prober.Device(), isPresent)
			if err != nil {
				log.Error().Err(err).Msgf("error probing %s", name)
				if !sendPayloadOnError {
					return
				}
			}

			msg := translateOutcome(isPresent)
			if err := app.notifier.Notify(ctx, prober.Device(), msg); err != nil {
				log.Error().Err(err).Msgf("error dispatching updates for %s: %v", name, err)
			}

			w.Done()
		}(name, probe, wg)
	}

	wg.Wait()
}

func parseFlags() {
	flag.StringVar(&flagConfigFile, "config", defaultConfigFile, "config file")
	flag.BoolVar(&flagVersion, "version", false, "print version and exit")

	flag.Parse()

	if flagVersion {
		fmt.Println(BuildVersion)
		os.Exit(0)
	}
}

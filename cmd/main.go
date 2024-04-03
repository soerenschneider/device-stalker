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

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/soerenschneider/device-stalker/internal"
	"golang.org/x/term"
)

const (
	AppName = "device-stalker"
)

var (
	defaultConfigFile = fmt.Sprintf("/etc/%s.yaml", AppName)

	flagConfigFile string
	flagVersion    bool
	flagDebug      bool

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

type DeviceState interface {
	HasStateChanged(id string, newState bool) bool
}

type App struct {
	samplers map[string]Sampler
	notifier Notifier
	state    DeviceState
	config   *internal.Config
}

func main() {
	parseFlags()
	initLogging(flagDebug)

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

	stateImpl, err := buildState(conf)
	if err != nil {
		log.Fatal().Err(err).Msgf("could not build state backend")
	}

	go func() {
		internal.VersionMetric.WithLabelValues(BuildVersion, CommitHash).Set(1)
		internal.ProcessStartTime.SetToCurrentTime()
		internal.Heartbeat.SetToCurrentTime()
		if err := internal.StartMetricsServer(conf.MetricsAddr); err != nil {
			log.Fatal().Err(err).Msg("can not start metrics server")
		}
	}()

	app := &App{
		samplers: samplers,
		notifier: notifier,
		state:    stateImpl,
		config:   conf,
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
			internal.Heartbeat.SetToCurrentTime()
			app.tick(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (app *App) evaluateState(ctx context.Context, id string, isPresent bool, device internal.Device) {
	stateChanged := app.state.HasStateChanged(id, isPresent)

	if stateChanged {
		internal.DeviceStatusChanges.WithLabelValues(id, device.Target).Inc()
		log.Info().Str("id", id).Bool("isPresent", isPresent).Msgf("Detected state change")
	}

	if !stateChanged && !app.config.AlwaysSendNotification {
		// if the state has not changed and we only want to send state data when on changes, return here
		return
	}

	msg := translateOutcome(isPresent)
	if err := app.notifier.Notify(ctx, device, msg); err != nil {
		log.Error().Err(err).Msgf("error dispatching updates for %s: %v", id, err)
	}
}

func (app *App) tick(ctx context.Context) {
	wg := &sync.WaitGroup{}
	for name, probe := range app.samplers {
		wg.Add(1)
		go func(name string, prober Sampler, w *sync.WaitGroup) {
			isPresent, err := prober.Check(ctx)

			if err != nil {
				log.Error().Err(err).Msgf("error probing %s", name)
			} else {
				if isPresent {
					internal.DeviceStatusPresent.WithLabelValues(name, prober.Device().Target).Set(1)
				} else {
					internal.DeviceStatusPresent.WithLabelValues(name, prober.Device().Target).Set(0)
				}
				log.Debug().Msgf("Probed %v=%t", prober.Device(), isPresent)
				app.evaluateState(ctx, name, isPresent, prober.Device())
			}

			w.Done()
		}(name, probe, wg)
	}

	wg.Wait()
}

func parseFlags() {
	flag.StringVar(&flagConfigFile, "config", defaultConfigFile, "config file")
	flag.BoolVar(&flagVersion, "version", false, "print version and exit")
	flag.BoolVar(&flagDebug, "debug", false, "print debug logs")

	flag.Parse()

	if flagVersion {
		fmt.Println(BuildVersion)
		os.Exit(0)
	}
}

func initLogging(debug bool) {
	if term.IsTerminal(int(os.Stdout.Fd())) {
		log.Logger = log.Output(zerolog.ConsoleWriter{
			Out:        os.Stderr,
			TimeFormat: "15:04:05",
		})
	}

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	log.Info().Str("version", BuildVersion).Str("commit", CommitHash).Msgf("Started %s", AppName)
}

func translateOutcome(isPresent bool) string {
	if isPresent {
		return "ON"
	}

	return "OFF"
}

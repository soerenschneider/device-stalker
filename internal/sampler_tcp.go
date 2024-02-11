package internal

import (
	"context"
	"net"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"go.uber.org/multierr"
)

const TcpPrefix = "tcp://"

type TcpSampler struct {
	probe   Device
	timeout time.Duration
	count   int
}

func NewTcpSampler(conf TcpSamplerConfig, probe Device) (*TcpSampler, error) {
	p := &TcpSampler{
		timeout: time.Duration(conf.TimeoutSeconds) * time.Second,
		probe:   probe,
	}

	p.probe.Target = strings.TrimPrefix(p.probe.Target, TcpPrefix)
	return p, nil
}

func (p *TcpSampler) Device() Device {
	return p.probe
}

func (p *TcpSampler) Check(_ context.Context) (bool, error) {
	var errs error
	for i := 0; i < p.count; i++ {
		success, err := check(p.probe.Target, p.timeout)
		if err != nil {
			errs = multierr.Append(errs, err)
		} else if success {
			return true, nil
		}
	}

	return false, errs
}

func check(address string, timeout time.Duration) (bool, error) {
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return false, err
	}

	defer func() {
		if err := conn.Close(); err != nil {
			log.Error().Err(err).Msg("could not close tcp connection")
		}
	}()

	return true, nil
}

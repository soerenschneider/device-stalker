package internal

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	probing "github.com/prometheus-community/pro-bing"
	"github.com/rs/zerolog/log"
)

const IcmpPrefix = "icmp://"

type IcmpSampler struct {
	conf   IcmpSamplerConfig
	device Device
	name   string
}

func NewIcmpSampler(conf IcmpSamplerConfig, device Device) (*IcmpSampler, error) {
	device.Target = strings.TrimPrefix(device.Target, IcmpPrefix)

	ret := &IcmpSampler{
		conf:   conf,
		name:   device.Name,
		device: device,
	}

	return ret, nil
}

func (p *IcmpSampler) Device() Device {
	return p.device
}

func (p *IcmpSampler) Check(_ context.Context) (bool, error) {
	pinger, err := probing.NewPinger(p.device.Target)
	dnsErr := &net.DNSError{}
	if err != nil {
		if errors.As(err, &dnsErr) {
			log.Warn().Err(dnsErr).Msgf("could not resolve host")
		} else {
			return false, fmt.Errorf("could not build device for %w", err)
		}
	}

	pinger.Count = p.conf.Count
	pinger.Timeout = time.Duration(p.conf.TimeoutSeconds) * time.Second
	pinger.SetPrivileged(p.conf.UsePrivileged)

	if err := pinger.Run(); err != nil {
		SampleErrors.WithLabelValues("ping", p.name, p.device.Target).Inc()
		return false, err
	}

	return pinger.Statistics().PacketsRecv > 0, nil
}

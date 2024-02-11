package main

import (
	"fmt"
	"strings"

	"github.com/soerenschneider/device-stalker/internal"
)

func buildNotifier(conf *internal.Config) (Notifier, error) {
	mqtt, err := internal.NewMqttClient(conf.Mqtt.Broker, conf.Mqtt.ClientId, conf.Mqtt.DefaultTopic, conf.Mqtt.TlsConfig())
	if err != nil {
		return nil, err
	}

	return mqtt, nil
}

func buildSamplers(conf *internal.Config) (map[string]Sampler, error) {
	ret := map[string]Sampler{}

	for _, probe := range conf.Devices {
		p, err := buildSampler(conf, probe)
		if err != nil {
			return nil, err
		}
		ret[probe.Name] = p
	}

	return ret, nil
}

func buildSampler(conf *internal.Config, probe internal.Device) (Sampler, error) {
	if strings.HasPrefix(probe.Target, internal.TcpPrefix) {
		return internal.NewTcpSampler(conf.TcpSampler, probe)
	}
	if strings.HasPrefix(probe.Target, internal.IcmpPrefix) {
		return internal.NewIcmpSampler(conf.IcmpSampler, probe)
	}
	return nil, fmt.Errorf("don't know how to build %q", probe.Target)
}

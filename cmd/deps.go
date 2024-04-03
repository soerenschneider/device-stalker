package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/soerenschneider/device-stalker/internal"
	"github.com/soerenschneider/device-stalker/pkg"
)

func buildNotifier(conf *internal.Config) (Notifier, error) {
	return buildMqttNotifier(conf)
}

func buildMqttNotifier(conf *internal.Config) (*internal.MqttClientBus, error) {
	clientId := conf.Mqtt.ClientId
	if len(clientId) == 0 {
		hostname, err := os.Hostname()
		if err != nil {
			return nil, fmt.Errorf("automatically setting mqtt clientId failed: %w", err)
		}
		clientId = fmt.Sprintf("%s-%s", AppName, hostname)
	}

	if conf.Mqtt.RandomClientIdSuffix {
		clientId = fmt.Sprintf("%s-%s", conf.Mqtt.ClientId, pkg.GenRandomSuffix())
	}

	mqtt, err := internal.NewMqttClient(conf.Mqtt.Broker, clientId, conf.Mqtt.DefaultTopic, conf.Mqtt.TlsConfig())
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

func buildState(conf *internal.Config) (DeviceState, error) {
	return internal.NewMapState()
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

package internal

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/rs/zerolog/log"
)

const (
	defaultQos byte = 1
)

var (
	publishWaitTimeout = 5 * time.Second
	mutex              sync.Mutex
)

type MqttClientBus struct {
	client            mqtt.Client
	notificationTopic string
}

func NewMqttClient(broker string, clientId, notificationTopic string, tlsConfig *tls.Config) (*MqttClientBus, error) {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetClientID(clientId)
	if tlsConfig != nil {
		opts.SetTLSConfig(tlsConfig)
	}

	opts.SetAutoReconnect(true)
	opts.SetMaxReconnectInterval(60 * time.Second)
	opts.SetConnectRetry(true)
	opts.SetClientID(clientId)

	opts.OnConnectionLost = connectLostHandler
	opts.OnConnectAttempt = onConnectAttemptHandler
	opts.OnConnect = onConnectHandler
	opts.OnReconnecting = onReconnectHandler

	client := mqtt.NewClient(opts)
	token := client.Connect()
	finishedWithinTimeout := token.WaitTimeout(10 * time.Second)
	if token.Error() != nil || !finishedWithinTimeout {
		log.Error().Err(token.Error()).Msgf("Connection to broker %q failed, continuing in background", broker)
	}

	return &MqttClientBus{
		client:            client,
		notificationTopic: notificationTopic,
	}, nil
}

func (d *MqttClientBus) Notify(_ context.Context, probe Device, val string) error {
	topic := probe.Topic

	if len(topic) == 0 {
		// fall back to default topic
		topic = fmt.Sprintf(d.notificationTopic, probe.Name)
	}

	token := d.client.Publish(topic, defaultQos, false, val)
	ok := token.WaitTimeout(publishWaitTimeout)
	if !ok {
		NotificationErrors.Inc()
		return errors.New("received timeout when trying to publish the message")
	}

	return nil
}

func connectLostHandler(client mqtt.Client, err error) {
	opts := client.OptionsReader()
	log.Info().Msgf("Connection lost from %v: %v", opts.Servers(), err)
	MqttConnectionsLostTotal.Inc()
	mutex.Lock()
	defer mutex.Unlock()
	MqttBrokersConnectedTotal.Sub(1)
}

func onReconnectHandler(_ mqtt.Client, opts *mqtt.ClientOptions) {
	mutex.Lock()
	MqttReconnectionsTotal.Inc()
	mutex.Unlock()
	log.Info().Msgf("Reconnecting to %s", opts.Servers)
}

func onConnectAttemptHandler(broker *url.URL, tlsCfg *tls.Config) *tls.Config {
	log.Info().Msgf("Attempting to connect to broker %s", broker.Host)
	return tlsCfg
}

var onConnectHandler = func(c mqtt.Client) {
	opts := c.OptionsReader()
	log.Info().Msgf("Connected to broker(s) %v", opts.Servers())
	mutex.Lock()
	MqttBrokersConnectedTotal.Add(1)
	mutex.Unlock()
}

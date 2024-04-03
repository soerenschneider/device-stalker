package internal

import (
	"crypto/tls"
	"crypto/x509"
	"net/url"
	"os"
	"reflect"
	"runtime"
	"sync"

	"github.com/caarlos0/env/v10"
	"github.com/go-playground/validator/v10"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

var (
	once     sync.Once
	validate *validator.Validate
)

type Config struct {
	IcmpSampler IcmpSamplerConfig `yaml:"pinger" envPrefix:"PINGER_"`
	TcpSampler  TcpSamplerConfig  `yaml:"tcp" envPrefix:"TCP_"`

	Devices []Device `yaml:"devices" validate:"required,dive"`

	AlwaysSendNotification bool       `yaml:"always_send_notification" env:"ALWAYS_SEND_NOTIFICATION"`
	Mqtt                   MqttConfig `yaml:"mqtt" envPrefix:"MQTT_"`
	MetricsAddr            string     `yaml:"metrics_addr" env:"METRICS_ADDR" validate:"tcp_addr"`
}

type Device struct {
	Name   string `yaml:"name" validate:"required"`
	Target string `yaml:"target" validate:"required"`
	Topic  string `yaml:"topic"`
}

type TcpSamplerConfig struct {
	Count          int `yaml:"count" validate:"min=1,max=5"`
	TimeoutSeconds int `yaml:"timeout_s" validate:"min=1,max=5"`
}

type IcmpSamplerConfig struct {
	UsePrivileged  bool `yaml:"use_privileged" env:"PRIVILEGED"`
	Count          int  `yaml:"count" env:"COUNT" validate:"min=1,max=5"`
	TimeoutSeconds int  `yaml:"timeout_s" env:"TIMEOUT_S" validate:"min=1,max=5"`
}

type MqttConfig struct {
	Broker               string `yaml:"broker" env:"BROKER" validate:"broker"`
	DefaultTopic         string `yaml:"default_topic" env:"DEFAULT_TOPIC" validate:"required"`
	ClientId             string `yaml:"client_id" env:"CLIENT_ID" validate:"required"`
	RandomClientIdSuffix bool   `yaml:"random_client_id_suffix" env:"CLIENT_ID_RAND_SUFFIX"`

	CaCertFile     string `yaml:"tls_ca_cert" env:"TLS_CA_CERT_FILE" validate:"omitempty,file"`
	ClientCertFile string `yaml:"tls_client_cert" env:"TLS_CLIENT_CERT_FILE" validate:"omitempty,file"`
	ClientKeyFile  string `yaml:"tls_client_key" env:"TLS_CLIENT_KEY_FILE" validate:"omitempty,file"`
	TlsInsecure    bool   `yaml:"tls_insecure"`
}

func defaultConfig() Config {
	return Config{
		MetricsAddr: "127.0.0.1:9224",
		IcmpSampler: IcmpSamplerConfig{
			UsePrivileged:  runtime.GOOS == "linux",
			Count:          3,
			TimeoutSeconds: 1,
		},
		TcpSampler: TcpSamplerConfig{
			Count:          3,
			TimeoutSeconds: 1,
		},
	}
}

func ValidateConfig[T any](c T) error {
	once.Do(func() {
		validate = validator.New()
		if err := validate.RegisterValidation("broker", validateBroker); err != nil {
			log.Fatal().Err(err).Msg("could not build custom validation 'validateBroker'")
		}
	})

	return validate.Struct(c)
}

func ReadConfig(file string) (*Config, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}

	conf := defaultConfig()
	if err := yaml.Unmarshal(data, &conf); err != nil {
		return nil, err
	}

	opts := env.Options{
		Prefix: "DEVICE_STALKER_",
	}
	if err := env.ParseWithOptions(&conf, opts); err != nil {
		return nil, err
	}

	return &conf, nil
}

func (conf *MqttConfig) UsesTlsClientCerts() bool {
	return len(conf.CaCertFile) > 0 && len(conf.ClientCertFile) > 0 && len(conf.ClientKeyFile) > 0
}

func (conf *MqttConfig) TlsConfig() *tls.Config {
	certPool, err := x509.SystemCertPool()
	if err != nil {
		log.Warn().Msgf("Could not get system cert pool")
		certPool = x509.NewCertPool()
	}

	if conf.UsesTlsClientCerts() {
		pemCerts, err := os.ReadFile(conf.CaCertFile)
		if err != nil {
			log.Error().Msgf("Could not read CA cert file: %v", err)
		} else {
			certPool.AppendCertsFromPEM(pemCerts)
		}
	}

	tlsConf := &tls.Config{
		RootCAs:            certPool,
		ClientAuth:         tls.RequestClientCert,
		InsecureSkipVerify: conf.TlsInsecure, // #nosec G402
	}

	clientCertDefined := len(conf.ClientCertFile) > 0
	clientKeyDefined := len(conf.ClientKeyFile) > 0
	if clientCertDefined && clientKeyDefined {
		tlsConf.GetClientCertificate = func(info *tls.CertificateRequestInfo) (*tls.Certificate, error) {
			cert, err := tls.LoadX509KeyPair(conf.ClientCertFile, conf.ClientKeyFile)
			return &cert, err
		}
	}

	return tlsConf
}

func validateBroker(fl validator.FieldLevel) bool {
	field := fl.Field()
	if field.Kind() != reflect.String {
		return false
	}

	str, ok := field.Interface().(string)
	if !ok || !IsValidUrl(str) {
		return false
	}

	return true
}

func IsValidUrl(input string) bool {
	_, err := url.ParseRequestURI(input)
	if err != nil {
		return false
	}

	u, err := url.Parse(input)
	if err != nil || u.Scheme == "" || u.Host == "" || u.Port() == "" {
		return false
	}

	return true
}

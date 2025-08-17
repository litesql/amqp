package extension

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/walterwanderley/sqlite"

	"github.com/litesql/amqp/config"
)

type PublisherModule struct {
}

func (m *PublisherModule) Connect(conn *sqlite.Conn, args []string, declare func(string) error) (sqlite.VirtualTable, error) {
	virtualTableName := args[2]
	if virtualTableName == "" {
		virtualTableName = config.DefaultPublisherVTabName
	}

	var (
		cfg config.Config

		certFilePath    string
		certKeyFilePath string
		caFilePath      string
		insecure        bool

		err    error
		logger string
	)
	if len(args) > 3 {
		for _, opt := range args[3:] {
			k, v, ok := strings.Cut(opt, "=")
			if !ok {
				return nil, fmt.Errorf("invalid option: %q", opt)
			}
			k = strings.TrimSpace(k)
			v = sanitizeOptionValue(v)

			switch strings.ToLower(k) {
			case config.URI:
				cfg.URI = v
			case config.QueueDeleteUnsused:
				cfg.QueueDeleteUnsused, err = strconv.ParseBool(v)
				if err != nil {
					return nil, fmt.Errorf("invalid %q option value: %w", k, err)
				}
			case config.QueueDurable:
				cfg.QueueDurable, err = strconv.ParseBool(v)
				if err != nil {
					return nil, fmt.Errorf("invalid %q option value: %w", k, err)
				}
			case config.QueueExclusive:
				cfg.QueueExclusive, err = strconv.ParseBool(v)
				if err != nil {
					return nil, fmt.Errorf("invalid %q option value: %w", k, err)
				}
			case config.QueueNoWait:
				cfg.QueueNoWait, err = strconv.ParseBool(v)
				if err != nil {
					return nil, fmt.Errorf("invalid %q option value: %w", k, err)
				}
			case config.Timeout:
				i, err := strconv.Atoi(v)
				if err != nil {
					return nil, fmt.Errorf("invalid %q option value: %w", k, err)
				}
				cfg.Timeout = time.Duration(i) * time.Millisecond
			case config.CertFile:
				certFilePath = v
			case config.CertKeyFile:
				certKeyFilePath = v
			case config.CertCAFile:
				caFilePath = v
			case config.Insecure:
				insecure, err = strconv.ParseBool(v)
				if err != nil {
					return nil, fmt.Errorf("invalid %q option: %v", k, err)
				}
			case config.Logger:
				logger = v
			}
		}
	}

	tlsConfig := tls.Config{
		InsecureSkipVerify: insecure,
	}

	if certFilePath != "" && certKeyFilePath != "" {
		clientCert, err := tls.LoadX509KeyPair(certFilePath, certKeyFilePath)
		if err != nil {
			return nil, fmt.Errorf("error loading client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{clientCert}
	}

	if caFilePath != "" {
		caCertPEM, err := os.ReadFile(caFilePath)
		if err != nil {
			return nil, fmt.Errorf("error loading CA certificate: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCertPEM) {
			return nil, fmt.Errorf("error appending CA certificate to pool")
		}
		tlsConfig.RootCAs = caCertPool
	}

	cfg.AMQPConfig.TLSClientConfig = &tlsConfig

	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Second
	}

	vtab, err := NewPublisherVirtualTable(virtualTableName, cfg, logger)
	if err != nil {
		return nil, err
	}

	return vtab,
		declare("CREATE TABLE x(queue TEXT, content_type TEXT, body BLOB, mandatory INTEGER, immediate INTEGER)")
}

func sanitizeOptionValue(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "'")
	v = strings.TrimSuffix(v, "'")
	v = strings.TrimPrefix(v, "\"")
	v = strings.TrimSuffix(v, "\"")
	return os.ExpandEnv(v)
}

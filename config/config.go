package config

import (
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	// Common config
	URI                = "uri"                 // AMQP URI to connect to the broker (ex: amqp://user:pass@localhost:5672/)
	Timeout            = "timeout"             // timeout in milliseconds (default 5000)
	Consumer           = "consumer"            // Consumer
	QueueDurable       = "queue.durable"       // Queue durable config
	QueueDeleteUnsused = "queue.delete-unused" // Queue delete unused
	QueueExclusive     = "queue.exclusive"     // Queue exclusive
	QueueNoWait        = "queue.no-wait"       // Queue no wait
	CertFile           = "cert_file"           // TLS: path to certificate file
	CertKeyFile        = "cert_key_file"       // TLS: path to .pem certificate key file
	CertCAFile         = "ca_file"             // TLS: path to CA certificate file
	Insecure           = "insecure"            // TLS: Insecure skip TLS verification
	Logger             = "logger"              // Log errors to "stdout, stderr or file:/path/to/log.txt"

	// Subscribe module config
	TableName = "table" // table name where to store the incoming messages

	DefaultTableName          = "amqp_data"
	DefaultPublisherVTabName  = "amqp_pub"
	DefaultSubscriberVTabName = "amqp_sub"
)

type Config struct {
	URI                string
	AMQPConfig         amqp.Config
	QueueDurable       bool
	QueueDeleteUnsused bool
	QueueExclusive     bool
	QueueNoWait        bool
	Timeout            time.Duration
	Consumer           string
}

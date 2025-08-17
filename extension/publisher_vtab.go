package extension

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"

	"github.com/litesql/amqp/config"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/walterwanderley/sqlite"
)

type PublisherVirtualTable struct {
	conn         *amqp.Connection
	channel      *amqp.Channel
	config       config.Config
	name         string
	logger       *slog.Logger
	loggerCloser io.Closer
}

func NewPublisherVirtualTable(name string, config config.Config, loggerDef string) (*PublisherVirtualTable, error) {

	var (
		conn    *amqp.Connection
		channel *amqp.Channel
		err     error
	)
	if config.URI != "" {
		conn, err = amqp.DialConfig(config.URI, config.AMQPConfig)
		if err != nil {
			return nil, fmt.Errorf("connecting to amqp server: %w", err)
		}
		channel, err = conn.Channel()
		if err != nil {
			return nil, fmt.Errorf("open a channel to amqp server: %w", err)
		}
	}

	logger, loggerCloser, err := loggerFromConfig(loggerDef)
	if err != nil {
		return nil, err
	}

	return &PublisherVirtualTable{
		name:         name,
		conn:         conn,
		channel:      channel,
		config:       config,
		logger:       logger,
		loggerCloser: loggerCloser,
	}, nil
}

func (vt *PublisherVirtualTable) BestIndex(in *sqlite.IndexInfoInput) (*sqlite.IndexInfoOutput, error) {
	return &sqlite.IndexInfoOutput{}, nil
}

func (vt *PublisherVirtualTable) Open() (sqlite.VirtualCursor, error) {
	return nil, fmt.Errorf("SELECT operations on %q is not supported", vt.name)
}

func (vt *PublisherVirtualTable) Disconnect() error {
	var err error
	if vt.loggerCloser != nil {
		err = vt.loggerCloser.Close()
	}
	if vt.channel != nil {
		err = errors.Join(err, vt.channel.Close())
	}
	if vt.conn != nil {
		err = errors.Join(err, vt.conn.Close())
	}
	return err
}

func (vt *PublisherVirtualTable) Destroy() error {
	return nil
}

func (vt *PublisherVirtualTable) Insert(values ...sqlite.Value) (int64, error) {
	if vt.conn == nil {
		return 0, fmt.Errorf("not connected to the broker")
	}

	queue := values[0].Text()
	_, err := vt.channel.QueueDeclare(
		queue,
		vt.config.QueueDurable,
		vt.config.QueueDeleteUnsused,
		vt.config.QueueExclusive,
		vt.config.QueueNoWait,
		nil, // arguments
	)
	if err != nil {
		return 0, fmt.Errorf("failed to declare queue: %w", err)
	}

	contentType := values[1].Text()
	body := values[2].Text()
	var mandatory bool
	if values[3].Text() != "" {
		mandatory, err = strconv.ParseBool(values[3].Text())
		if err != nil {
			return 0, fmt.Errorf("invalid 'mandatory' value: %w", err)
		}
	}
	var immediate bool
	if values[4].Text() != "" {
		immediate, err = strconv.ParseBool(values[5].Text())
		if err != nil {
			return 0, fmt.Errorf("invalid 'immediate' value: %w", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), vt.config.Timeout)
	defer cancel()

	err = vt.channel.PublishWithContext(ctx,
		"", // exchange
		queue,
		mandatory,
		immediate,
		amqp.Publishing{
			ContentType: contentType,
			Body:        []byte(body),
		})

	if err != nil {
		return 0, fmt.Errorf("failed to publish message: %w", err)
	}
	return 1, nil
}

func (vt *PublisherVirtualTable) Update(_ sqlite.Value, _ ...sqlite.Value) error {
	return fmt.Errorf("UPDATE operations on %q is not supported", vt.name)
}

func (vt *PublisherVirtualTable) Replace(old sqlite.Value, new sqlite.Value, _ ...sqlite.Value) error {
	return fmt.Errorf("UPDATE operations on %q is not supported", vt.name)
}

func (vt *PublisherVirtualTable) Delete(_ sqlite.Value) error {
	return fmt.Errorf("DELETE operations on %q is not supported", vt.name)
}

package extension

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strconv"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/walterwanderley/sqlite"

	"github.com/litesql/amqp/config"
)

type SubscriberVirtualTable struct {
	virtualTableName string
	tableName        string
	conn             *amqp.Connection
	channel          *amqp.Channel
	config           config.Config
	subscriptions    []subscription
	stmt             *sqlite.Stmt
	stmtMu           sync.Mutex
	mu               sync.Mutex
	logger           *slog.Logger
	loggerCloser     io.Closer
}

type subscription struct {
	queue   string
	autoAck bool
	noLocal bool
}

func NewSubscriberVirtualTable(virtualTableName string, config config.Config, tableName string, conn *sqlite.Conn, loggerDef string) (*SubscriberVirtualTable, error) {

	stmt, _, err := conn.Prepare(fmt.Sprintf(`INSERT INTO %s(queue, message_id, content_type, body, timestamp) VALUES(?, ?, ?, ?, ?)`, tableName))
	if err != nil {
		return nil, err
	}

	var (
		amqpConn *amqp.Connection
		channel  *amqp.Channel
	)
	if config.URI != "" {
		amqpConn, err = amqp.DialConfig(config.URI, config.AMQPConfig)
		if err != nil {
			return nil, fmt.Errorf("connecting to amqp server: %w", err)
		}
		channel, err = amqpConn.Channel()
		if err != nil {
			return nil, fmt.Errorf("open a channel to amqp server: %w", err)
		}
	}

	logger, loggerCloser, err := loggerFromConfig(loggerDef)
	if err != nil {
		return nil, err
	}

	return &SubscriberVirtualTable{
		virtualTableName: virtualTableName,
		tableName:        tableName,
		subscriptions:    make([]subscription, 0),
		stmt:             stmt,
		conn:             amqpConn,
		channel:          channel,
		logger:           logger,
		loggerCloser:     loggerCloser,
	}, nil
}

func (vt *SubscriberVirtualTable) BestIndex(in *sqlite.IndexInfoInput) (*sqlite.IndexInfoOutput, error) {
	return &sqlite.IndexInfoOutput{EstimatedCost: 1000000}, nil
}

func (vt *SubscriberVirtualTable) Open() (sqlite.VirtualCursor, error) {
	return newSubscriptionsCursor(vt.subscriptions), nil
}

func (vt *SubscriberVirtualTable) Disconnect() error {
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

	return errors.Join(err, vt.stmt.Finalize())
}

func (vt *SubscriberVirtualTable) Destroy() error {
	return nil
}

func (vt *SubscriberVirtualTable) Insert(values ...sqlite.Value) (int64, error) {
	if vt.channel == nil {
		return 0, fmt.Errorf("not connected to the broker")
	}
	queue := values[0].Text()
	if queue == "" {
		return 0, fmt.Errorf("queue is required")
	}
	var err error
	var autoAck bool
	if values[1].Text() != "" {
		autoAck, err = strconv.ParseBool(values[1].Text())
		if err != nil {
			return 0, fmt.Errorf("invalid 'auto_ack' value: %w", err)
		}
	}
	var noLocal bool
	if values[2].Text() != "" {
		autoAck, err = strconv.ParseBool(values[2].Text())
		if err != nil {
			return 0, fmt.Errorf("invalid 'no_local' value: %w", err)
		}
	}

	vt.mu.Lock()
	defer vt.mu.Unlock()
	if vt.contains(queue) {
		return 0, fmt.Errorf("already subscribed to the %q queue", queue)
	}

	_, err = vt.channel.QueueDeclare(
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

	msgs, err := vt.channel.Consume(
		queue,
		vt.config.Consumer,
		autoAck,
		vt.config.QueueExclusive,
		noLocal,
		vt.config.QueueNoWait,
		nil, // args
	)
	if err != nil {
		return 0, fmt.Errorf("failed to consume: %w", err)
	}

	go func() {
		for msg := range msgs {
			vt.insertMessage(msg)
		}
	}()

	vt.subscriptions = append(vt.subscriptions, subscription{
		queue:   queue,
		autoAck: autoAck,
		noLocal: noLocal,
	})
	return 1, nil
}

func (vt *SubscriberVirtualTable) Update(_ sqlite.Value, _ ...sqlite.Value) error {
	return fmt.Errorf("UPDATE operations on %q is not supported", vt.virtualTableName)
}

func (vt *SubscriberVirtualTable) Replace(old sqlite.Value, new sqlite.Value, _ ...sqlite.Value) error {
	return fmt.Errorf("UPDATE operations on %q is not supported", vt.virtualTableName)
}

func (vt *SubscriberVirtualTable) Delete(v sqlite.Value) error {
	return fmt.Errorf("DELETE operations on %q is not supported", vt.virtualTableName)
}

func (vt *SubscriberVirtualTable) contains(queue string) bool {
	for _, subscription := range vt.subscriptions {
		if subscription.queue == queue {
			return true
		}
	}
	return false
}

func (vt *SubscriberVirtualTable) insertMessage(msg amqp.Delivery) {
	vt.stmtMu.Lock()
	defer vt.stmtMu.Unlock()
	err := vt.stmt.Reset()
	if err != nil {
		vt.logger.Error("reset statement", "error", err)
		return
	}
	vt.stmt.BindText(1, msg.RoutingKey)
	vt.stmt.BindText(2, msg.MessageId)
	vt.stmt.BindText(3, msg.ContentType)
	vt.stmt.BindText(4, string(msg.Body))
	vt.stmt.BindText(5, time.Now().Format(time.RFC3339Nano))
	if _, err := vt.stmt.Step(); err != nil {
		vt.logger.Error("insert data", "error", err, "queue", msg.RoutingKey, "message_id", msg.MessageId)
		return
	}
	if err := msg.Ack(false); err != nil {
		vt.logger.Error("ack", "error", err, "queue", msg.RoutingKey, "message_id", msg.MessageId)
	}
}

type subscriptionsCursor struct {
	data    []subscription
	current subscription // current row that the cursor points to
	rowid   int64        // current rowid .. negative for EOF
}

func newSubscriptionsCursor(data []subscription) *subscriptionsCursor {
	slices.SortFunc(data, func(a, b subscription) int {
		return cmp.Compare(a.queue, b.queue)
	})
	return &subscriptionsCursor{
		data: data,
	}
}

func (c *subscriptionsCursor) Next() error {
	// EOF
	if c.rowid < 0 || int(c.rowid) >= len(c.data) {
		c.rowid = -1
		return sqlite.SQLITE_OK
	}
	// slices are zero based
	c.current = c.data[c.rowid]
	c.rowid += 1

	return sqlite.SQLITE_OK
}

func (c *subscriptionsCursor) Column(ctx *sqlite.VirtualTableContext, i int) error {
	switch i {
	case 0:
		ctx.ResultText(c.current.queue)
	case 1:
		if c.current.autoAck {
			ctx.ResultInt(1)
		} else {
			ctx.ResultInt(0)
		}
	case 2:
		if c.current.noLocal {
			ctx.ResultInt(1)
		} else {
			ctx.ResultInt(0)
		}
	}
	return nil
}

func (c *subscriptionsCursor) Filter(int, string, ...sqlite.Value) error {
	c.rowid = 0
	return c.Next()
}

func (c *subscriptionsCursor) Rowid() (int64, error) {
	return c.rowid, nil
}

func (c *subscriptionsCursor) Eof() bool {
	return c.rowid < 0
}

func (c *subscriptionsCursor) Close() error {
	return nil
}

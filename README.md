# sqlite-amqp
SQLite Extension to integrate with AMQP protocol.

## Installation

Download **amqp** extension from the [releases page](https://github.com/litesql/amqp/releases).

### Compiling from source

- [Go 1.24+](https://go.dev) is required.

```sh
go build -ldflags="-s -w" -buildmode=c-shared -o amqp.so
```

## Basic usage

### Requirements

1. [SQLite3 cli](https://www.sqlite.org/download.html)

2. Access to any AMQP 0-9-1 protocol broker

```sh
docker run -d --name rabbitmq -p 5672:5672 -p 15672:15672 rabbitmq:3-management
```

### Loading the extension

```sh
sqlite3

# Load the extension
.load ./amqp

# check version (optional)
SELECT amqp_info();
```

### Subscriber

```sh
# Create a virtual table using AMQP_SUB to configure the connection to the broker
CREATE VIRTUAL TABLE temp.sub USING AMQP_SUB(uri='amqp://guest:guest@localhost:5672/', table=amqp_data);

# Insert the queue name into the created virtual table to subscribe
INSERT INTO temp.sub(queue) VALUES('my_queue');
```

Subscriber table schema:

```sql
TABLE temp.sub(
  queue TEXT,
  auto_ack INTEGER, -- 0 = false, 1 = true
  no_local INTEGER, -- 0 = false, 1 = true
)
```

### Publisher

```sh
# Create a virtual table using AMQP_PUB to configure the connection to the broker
CREATE VIRTUAL TABLE temp.pub USING AMQP_PUB(uri='amqp://guest:guest@localhost:5672/');

# Insert Queue, Content_Type and Body into the created virtual table to publish messages
INSERT INTO temp.pub(queue, content_type, body) VALUES('my_queue', 'text/plain', 'hello');
```

Publisher table schema:

```sql
TABLE temp.pub(
  queue TEXT, 
  content_type TEXT, 
  body BLOB, 
  mandatory INTEGER, -- 0 = false, 1 = true
  immediate INTEGER -- 0 = false, 1 = true
)
```

### Stored messages

```sh
# Set output mode (optional)
.mode qbox

# Query for the incoming messages
SELECT queue, content_type, body, timestamp FROM amqp_data;
┌────────────┬──────────────┬─────────┬───────────────────────────────────────┐
│   queue    │ content_type │  body   │               timestamp               │
├────────────┼──────────────┼─────────┼───────────────────────────────────────┤
│ 'my_queue' │ 'text/plain' │ 'hello' │ '2025-08-17T14:06:28.872790514-03:00' │
└────────────┴──────────────┴─────────┴───────────────────────────────────────┘
```

Incoming messages are stored in tables according to the following schema:

```sql
TABLE amqp_data(
  client_id TEXT,
  message_id INTEGER,
  topic TEXT,
  payload BLOB,
  qos INTEGER, -- 0, 1 or 2
  retained INTEGER, -- 0 = false, 1 = true
  timestamp DATETIME
)
```

### Subscriptions management

Query the subscription virtual table (the virtual table created using **amqp_sub**) to view all the active subscriptions for the current SQLite connection.

```sql
SELECT * FROM temp.sub;
┌────────────┬──────────┬──────────┐
│   queue    │ auto_ack │ no_local │
├────────────┼──────────┼──────────┤
│ 'my_queue' │ 0        │ 0        │
└────────────┴──────────┴──────────┘
```


## Configuring

You can configure the connection to the broker by passing parameters to the VIRTUAL TABLE.

| Param | Description | Default |
|-------|-------------|---------|
| uri | AMQP URI to connect to the broker (ex: amqp://user:pass@localhost:5672/) | |
| timeout | Timeout in milliseconds (default 5000) | |
| consumer | Consumer | |
| queue.durable | Queue durable config | false |
| queue.delete-unused | Queue delete unused | false |
| queue.exclusive | Queue exclusive | false |
| queue.no-wait | Queue no wait | false |
| cert_file | TLS: path to certificate file | |
| cert_key_file | TLS: path to .pem certificate key file | |
| ca_file | TLS: path to CA certificate file | |
| insecure | TLS: Insecure skip TLS verification | false |
| table | Name of the table where incoming messages will be stored. Only for amqp_sub | amqp_data |
| logger | Log errors to stdout, stderr or file:/path/to/file.log |

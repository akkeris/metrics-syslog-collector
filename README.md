# Metrics Syslog Collector

Add custom metrics to Influx/OpenTSDB via remote syslog

## Syslog Format

Syslog messages must be formatted according to `RFC5424` and sent via TCP. 

The message of the syslog should have the following format:

`type#metric.name=value`

Type can be "measure", "sample", or "count"

`metric.name` is a dot delimited description of the metric. The exact name is up to you and does not need to contain the Akkeris app name (that will be added automatically)

Examples:
- `measure#client.get=1ms`
- `sample#db.size=100MB`
- `count#email.sent=1`

## Requirements

Metrics Syslog Collector has one required dependency and one optional - an OpenTSDB compatible server (i.e. influxdb) is required, and a Postgres database is optional (although required for metric limiting)

## Environment Variables

For basic usage, only the following environment variables are required:

| Name                        | Description                                                               | Required  |
| --------------------------- | ------------------------------------------------------------------------- | --------- |
| OPENTSDB_IP                 | IP address of the OpenTSDB compatible server to send metrics to           | Required  |
| PORT                        | Network port to listen on                                                 | Required  |
| DEBUG                       | Set to "true" to print out additional degugging info                      | Optional  |

### Metric Limiting

If you wish to limit the number of unique metrics that apps can submit to Influx, you can use the following environment variables.

**NOTE:** A Postgres database is required for metric limiting

| Name                        | Description                                                                                     |
| --------------------------- | ----------------------------------------------------------------------------------------------- | 
| ENABLE_UNIQUE_METRIC_LIMIT  | Limit how many metrics each app can send to influx                                              |
| DATABASE_URL                | URL connection string for a Postgres database. **Required**                                     | 
| UNIQUE_METRIC_LIMIT         | How many metrics should be allowed per app? (default 100)                                       |
| LOGSHUTTLE_URL              | Set this to "true" to send rejection messages back to the app's logs                            |
| UNIQUE_METRIC_LIMIT_HELP    | Specify a link to the docs in the rejection message (optional)                                  |
| REJECT_MESSAGE_LIMIT        | Limit the number of times we report to the app logs (default 1) (reset on application restart)  | 

## Running

Docker:

```shell
$ docker build -t metrics-syslog-collector .
$ docker run --env-file config.env -p 9000:9000 metrics-syslog-collector
```

Locally:

```shell
$ source config.env
$ go run main.go
```

## Example

We do not have automated tests for this yet. However, it is possible to see how the app works by doing the following:

_(Note: these instructions were created using MacOS)_

### 1. Install influxdb and enable opentsdb

First, install influxdb:
```shell
$ brew install influxdb
```

Modify the influx configuration file:
```
$ vi /usr/local/etc/influxdb.conf
```

Find the `[[opentsdb]]` section and make the following changes to enable opentsdb:
```
489|  [[opentsdb]]
490|    enabled = true
491|    bind-address = ":4242"
492|    database = "opentsdb"
```

Start the influx service:
```shell
$ influxd -config /usr/local/etc/influxdb.conf
```

### 2. Install postgres and create a new database

```shell
$ brew install postgresql
$ brew services start postgresql
$ createdb metrics-syslog-collector
```

### 3. Start the Metrics Syslog Collector

Get your environment variables ready and put them in a file (`config.env`):
```
export ENABLE_UNIQUE_METRIC_LIMIT=true
export DATABASE_URL=postgres://[YOUR USER NAME]@127.0.0.1:5432/metrics-syslog-collector?sslmode=disable
export DEBUG=true
export OPENTSDB_IP=127.0.0.1:4242
export PORT=9000
```
_(Note: Be sure to replace [YOUR USER NAME] with your account username)_

Now, you can start the application:
```shell
$ source config.env
$ go run main.go
```

### 4. Send syslog message to Metrics Syslog Collector

There are a few ways to do this, but on MacOS, it's easiest to use Docker. Open up a new shell and start up a Debian image:

```shell
$ docker run --rm -it debian /bin/bash

$ logger -n host.docker.internal -P 9000 -t myapp -T "count#test=1"
```

You should see some chatter on the terminal window with Metrics Syslog Collector. 

### 5. Query Influx

Now, we can query Influx to see if the data was sent successfully:

```shell
$ curl http://127.0.0.1:8086/query\?db\=opentsdb\&q\=SHOW%20MEASUREMENTS
```

## Validation

Once this is is setup in your cluster, you can head to the `akkeris-system` namespace to see if this is working correctly. 

First, verify that you see the deployment in the namespace: 

```bash
# Verify that you see the deployment in the akkeris-system namespace 
kubectl get deployments | grep metrics-syslog-collector
metrics-syslog-collector      2/2     2            2           323d
# Check for active running pods 
kubectl get pods | grep metrics-syslog-collector
metrics-syslog-collector-5bfdb5d5c7-4rtxq      1/1     Running            0          4d18h
metrics-syslog-collector-5bfdb5d5c7-jnfcb      1/1     Running            0          4d18h
```

Finally, check that the logs are flowing in your pods. These logs are fairly verbose, so when you are tailing the logs the output should be substantial in a production environment. Here is a log excerpt for a functioning `metric-syslog-collector`: 

```bash
kubectl logs -f metrics-syslog-collector-<pod>-<id>

put count.request.status.200 1702000635911 1 app=give-perf-prd requested_client=greatwork dyno=dyno-8b8c6c687-444jb path=GET/eProduct/:eProductId

put count.request 1702000635912 1 app=give-perf-prd requested_client=greatwork dyno=dyno-8b8c6c687-444jb path=GET/eProduct/:eProductId

put measure.service 1702000635913 166 app=give-perf-prd requested_client=greatwork dyno=dyno-8b8c6c687-444jb path=GET/eProduct/:eProductId

put count.request.status.200 1702000635914 1 app=give-perf-prd requested_client=greatwork dyno=dyno-8b8c6c687-444jb path=GET/eProduct/:eProductId

put count.request 1702000635915 1 app=give-perf-prd requested_client=greatwork dyno=dyno-8b8c6c687-444jb path=GET/eProduct/:eProductId

put measure.request.identity.search 1702000636080 21 app=identity2-core-prd

put measure.es.identity.bulklookup 1702000636081 7 app=identity2-core-prd

put count.request.graphql.currentIdentity 1702000636082 1 app=identity2-core-prd

put measure.request.graphql.currentIdentity 1702000636083 14 app=identity2-core-prd

put measure.es.identity.bulklookup 1702000636085 7 app=identity2-core-prd
```
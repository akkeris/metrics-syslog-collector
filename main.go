package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq" //driver
	"gopkg.in/mcuadros/go-syslog.v2"
)

var conn net.Conn
var db *sql.DB

var uniqueMetricLimit int

const defaultMetricLimit int = 100

func initDB() (err error) {
	var dberr error
	db, dberr = sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if dberr != nil {
		return dberr
	}
	db.SetMaxIdleConns(4)
	db.SetMaxOpenConns(20)

	buf, err := ioutil.ReadFile("./create.sql")
	if err != nil {
		log.Println("Error: Unable to run migration scripts, could not load create.sql.")
		return err
	}

	_, err = db.Exec(string(buf))
	if err != nil {
		log.Println("Error: Unable to run migration scripts, execution failed.")
		return err
	}

	return nil
}

// checkMetric - Make sure that the app reporting the metric hasn't reached its unique metric limit
func checkMetric(app string, metric string) bool {
	// If in doubt (i.e. error returned) just send the metric to Influx

	// Does metric exist in the database?
	var exists bool
	err := db.QueryRow("select exists(select * from app_metrics where app = $1 and metric = $2 and deleted = 'f')", app, metric).Scan(&exists)
	if err != nil {
		fmt.Println(err)
		exists = true
	}

	if exists {
		return true
	}

	// Metric does not exist in the database, need to make sure the app is under the uniqueMetricLimit
	var metricCount int
	err = db.QueryRow("select count(*) from app_metrics where app = $1 and deleted = 'f'", app).Scan(&metricCount)
	if err != nil {
		fmt.Println(err)
		return true
	}

	if metricCount < uniqueMetricLimit {
		_, err := db.Exec("insert into app_metrics(app, metric) values($1, $2)", app, metric)
		if err != nil {
			fmt.Println(err)
		}
		return true
	}

	if os.Getenv("DEBUG") == "true" {
		fmt.Println("Did not send metric " + metric + " to influx for app " + app + ": Unique metric limit reached")
	}
	return false
}

func sendMetric(v []string, t map[string]string, message string, tags [][]string) {
	var finalval string
	finalval = v[3]
	if v[4] == "MB" && strings.Contains(message, "[metrics]") {
		val, err := strconv.ParseFloat(v[3], 64)
		if err != nil {
			fmt.Println(err)
		}
		finalval = strconv.FormatFloat(val*1048576, 'f', -1, 64)
	} else if v[4] == "GB" && strings.Contains(message, "[metrics]") {
		val, err := strconv.ParseFloat(v[3], 64)
		if err != nil {
			fmt.Println(err)
		}
		finalval = strconv.FormatFloat(val*1073741824, 'f', -1, 64)
	} else if v[4] == "KB" && strings.Contains(message, "[metrics]") {
		val, err := strconv.ParseFloat(v[3], 64)
		if err != nil {
			fmt.Println(err)
		}
		finalval = strconv.FormatFloat(val*1024, 'f', -1, 64)
	} else {
		finalval = v[3]
	}
	for _, tag := range tags {
		t[tag[1]] = tag[2]
	}
	t["metric"] = v[2]
	value, _ := strconv.ParseFloat(finalval, 64)
	fields := map[string]interface{}{
		"value": value,
	}
	tsdbmetric := v[1] + "." + t["metric"]
	tsdbtags := "app=" + t["app"]
	for _, tagelement := range tags {
		tsdbtags = tsdbtags + " " + tagelement[1] + "=" + tagelement[2]
	}
	tsdbvalue := fields["value"].(float64)
	s := strconv.FormatFloat(tsdbvalue, 'f', -1, 64)
	timestamp := strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10)
	put := "put " + tsdbmetric + " " + timestamp + " " + s + " " + tsdbtags + "\n"
	if os.Getenv("DEBUG") == "true" {
		fmt.Println(put)
	}
	fmt.Fprintf(conn, put)
}

func rejectMetric(app string, metric string, metricType string) {
	appParts := strings.SplitN(app, "-", 2)
	appName := appParts[0]
	appSpace := appParts[1]

	rejectMessage := struct {
		Log        string      `json:"log"`
		Stream     string      `json:"stream"`
		Time       time.Time   `json:"time"`
		Kubernetes interface{} `json:"kubernetes"`
		Topic      string      `json:"topic"`
	}{
		"Unique metrics limit exceeded. Metric discarded: [" + metricType + "] " + metric,
		"stdout",
		time.Now(),
		struct {
			PodName       string `json:"pod_name"`
			ContainerName string `json:"container_name"`
		}{
			"akkeris-warning",
			appName,
		},
		appSpace,
	}

	var logshuttleURL string
	if os.Getenv("LOGSHUTTLE_URL") != "" {
		logshuttleURL = os.Getenv("LOGSHUTTLE_URL")
	} else {
		logshuttleURL = "http://logshuttle.akkeris-system.svc.cluster.local"
	}

	jsonb, err := json.Marshal(rejectMessage)
	if err != nil {
		fmt.Println("Unable to send reject message to logshuttle")
		fmt.Println(err)
		return
	}

	resp, err := http.Post(logshuttleURL+"/log-events", "application/json", bytes.NewBuffer(jsonb))
	if err != nil {
		fmt.Println("Unable to send reject message to logshuttle")
		fmt.Println(err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		fmt.Println("Error: " + strconv.Itoa(resp.StatusCode) + " response returned from logshuttle")
	}
}

func main() {
	var err error
	conn, err = net.Dial("tcp", os.Getenv("OPENTSDB_IP"))
	if err != nil {
		fmt.Println("dial error:", err)
		return
	}

	channel := make(syslog.LogPartsChannel)
	handler := syslog.NewChannelHandler(channel)

	server := syslog.NewServer()
	server.SetFormat(syslog.RFC5424)
	server.SetHandler(handler)
	server.ListenTCP("0.0.0.0:" + os.Getenv("PORT"))

	server.Boot()

	err = initDB()
	if err != nil {
		fmt.Println("Error establishing database connection: " + err.Error())
		return
	}

	if os.Getenv("UNIQUE_METRIC_LIMIT") != "" {
		limit, err := strconv.ParseInt(os.Getenv("UNIQUE_METRIC_LIMIT"), 10, 0)
		if err != nil {
			uniqueMetricLimit = defaultMetricLimit
		} else {
			uniqueMetricLimit = int(limit)
		}
	}

	re, _ := regexp.Compile(`(measure|count|sample)#(\S*)=([0-9\.]+)(\S*)`)
	tagsRe, _ := regexp.Compile(` tag#(\S*)=(\S*)`)

	go func(channel syslog.LogPartsChannel) {
		for logParts := range channel {
			message, _ := logParts["message"].(string)
			if strings.Contains(message, "v-user-client-metrics") {
				continue
			}

			t := make(map[string]string)
			t["app"] = logParts["hostname"].(string)

			measurements := re.FindAllStringSubmatch(message, -1)
			tags := tagsRe.FindAllStringSubmatch(message, -1)

			for _, v := range measurements {
				t["metric"] = v[2]
				ok := checkMetric(t["app"], t["metric"])
				if ok {
					sendMetric(v, t, message, tags)
				} else {
					rejectMetric(t["app"], t["metric"], v[1])
				}
			}
		}
	}(channel)

	server.Wait()
}

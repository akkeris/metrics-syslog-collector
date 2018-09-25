package main

import (
	"fmt"
	client "github.com/influxdata/influxdb/client/v2"
	"gopkg.in/mcuadros/go-syslog.v2"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var c client.Client
var database_name string

func main() {
	var err error
	c, err = client.NewHTTPClient(client.HTTPConfig{
		Addr: os.Getenv("INFLUX_URL"),
	})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	database_name = os.Getenv("DATABASE_NAME")

	channel := make(syslog.LogPartsChannel)
	handler := syslog.NewChannelHandler(channel)

	server := syslog.NewServer()
	server.SetFormat(syslog.RFC5424)
	server.SetHandler(handler)
	server.ListenTCP("0.0.0.0:" + os.Getenv("PORT"))

	server.Boot()

	re, _ := regexp.Compile(`(measure|count|sample)#(\S*)=([0-9\.]+)(\S*)`)
	tagsRe, _ := regexp.Compile(` tag#(\S*)=(\S*)`)
	go func(channel syslog.LogPartsChannel) {
		for logParts := range channel {
			message, _ := logParts["message"].(string)
			t := make(map[string]string)
			t["app"] = logParts["hostname"].(string)
			measurements := re.FindAllStringSubmatch(message, -1)
			tags := tagsRe.FindAllStringSubmatch(message, -1)
			bp, err := client.NewBatchPoints(client.BatchPointsConfig{
				Database:  database_name,
				Precision: "us",
			})
			if err != nil {
				fmt.Println(err)
			}
			for _, v := range measurements {
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
				infmetric := v[1] + "." + t["metric"]
				inftags := "app=" + t["app"]
				var influxtags map[string]string
				influxtags = make(map[string]string)
				for _, tagelement := range tags {
					inftags = inftags + " " + tagelement[1] + "=" + tagelement[2]
					influxtags[tagelement[1]] = tagelement[2]
				}
				infvalue := fields["value"].(float64)
				s := strconv.FormatFloat(infvalue, 'f', -1, 64)
				influxfields := map[string]interface{}{
					"value": s,
				}
				pt, err := client.NewPoint(
					infmetric,
					influxtags,
					influxfields,
					time.Now(),
				)
				if err != nil {
					fmt.Println(err)
				}
				bp.AddPoint(pt)

			}
			go sendtoinflux(bp)
		}
	}(channel)

	server.Wait()

}

func sendtoinflux(bp client.BatchPoints) {
	if err := c.Write(bp); err != nil {
		fmt.Println(err)
	}

}

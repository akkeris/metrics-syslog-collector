package main

import (
    "fmt"
    "gopkg.in/mcuadros/go-syslog.v2"
    "net"
    "os"
    "regexp"
    "strconv"
    "strings"
    "time"
)

func main() {

    conn, err := net.Dial("tcp", os.Getenv("OPENTSDB_IP"))
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
                if os.Getenv("DEBUG")=="true" {
                    fmt.Println(put)
                }
                fmt.Fprintf(conn, put)
            }
        }
    }(channel)

    server.Wait()

}

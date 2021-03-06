package influx

import (
	"fmt"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/openziti/dilithium/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func init() {
	influxLoadCmd.Flags().BoolVarP(&influxLoadRetime, "retime", "r", false, "Shift metrics timestamps as close to 'now' as possible")
	influxCmd.AddCommand(influxLoadCmd)
}

var influxLoadCmd = &cobra.Command{
	Use:   "load <metricsRoot>",
	Short: "Load metrics data into the analyzer",
	Args:  cobra.ExactArgs(1),
	Run:   influxLoad,
}
var influxLoadRetime bool

func influxLoad(_ *cobra.Command, args []string) {
	authToken := ""
	if influxDbUsername != "" || influxDbPassword != "" {
		authToken = fmt.Sprintf("%s:%s", influxDbUsername, influxDbPassword)
	}
	client := influxdb2.NewClient(influxDbUrl, authToken)

	metricsMap, err := util.DiscoverMetrics(args[0])
	if err != nil {
		panic(err)
	}

	retimeMs := int64(0)
	if influxLoadRetime {
		newRetimeMs, err := scanForLatestTimestamp(metricsMap)
		if err != nil {
			panic(err)
		}
		retimeMs = newRetimeMs
	}

	for metricsRoot, metricsId := range metricsMap {
		switch metricsId.Id {
		case "westworld2.1":
			if err := loadWestworld21Metrics(metricsRoot, retimeMs, client); err != nil {
				panic(err)
			}

		case "westworld3.1":
			if err := loadWestworld31Metrics(metricsRoot, retimeMs, client); err != nil {
				panic(err)
			}

		case "dilithiumLoop":
			if err := loadDilithiumLoopMetrics(metricsRoot, retimeMs, client); err != nil {
				panic(err)
			}

		default:
			logrus.Warnf("unknown metrics type [%s]", metricsId.Id)
		}
	}

	client.Close()
}

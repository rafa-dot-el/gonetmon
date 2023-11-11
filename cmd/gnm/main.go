package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type NetworkStats struct {
	packetsTX uint64
	packetsRX uint64
	droppedTX uint64
	droppedRX uint64
	bytesTX   uint64
	bytesRX   uint64
}

func NewNetworkStats() *NetworkStats {
	return &NetworkStats{}
}

func (ns *NetworkStats) Read(interfaceName string) error {
	var err error

	if ns.packetsTX, err = readUint64FromFile(fmt.Sprintf(txPacketsPath, interfaceName)); err != nil {
		return err
	}

	if ns.packetsRX, err = readUint64FromFile(fmt.Sprintf(rxPacketsPath, interfaceName)); err != nil {
		return err
	}

	if ns.droppedTX, err = readUint64FromFile(fmt.Sprintf(txDroppedPath, interfaceName)); err != nil {
		return err
	}

	if ns.droppedRX, err = readUint64FromFile(fmt.Sprintf(rxDroppedPath, interfaceName)); err != nil {
		return err
	}

	if ns.bytesTX, err = readUint64FromFile(fmt.Sprintf(txBytesPath, interfaceName)); err != nil {
		return err
	}

	if ns.bytesRX, err = readUint64FromFile(fmt.Sprintf(rxBytesPath, interfaceName)); err != nil {
		return err
	}

	return nil
}

func (ns *NetworkStats) NormalizeDiff(other *NetworkStats, divisor uint64) *NetworkStats {
	diff := &NetworkStats{
		packetsTX: (other.packetsTX - ns.packetsTX) / divisor,
		packetsRX: (other.packetsRX - ns.packetsRX) / divisor,
		droppedTX: (other.droppedTX - ns.droppedTX) / divisor,
		droppedRX: (other.droppedRX - ns.droppedRX) / divisor,
		bytesTX:   (other.bytesTX - ns.bytesTX) / divisor,
		bytesRX:   (other.bytesRX - ns.bytesRX) / divisor,
	}
	return diff
}

const (
	rxPacketsPath   = "/sys/class/net/%s/statistics/rx_packets"
	txPacketsPath   = "/sys/class/net/%s/statistics/tx_packets"
	rxBytesPath     = "/sys/class/net/%s/statistics/rx_bytes"
	txBytesPath     = "/sys/class/net/%s/statistics/tx_bytes"
	rxDroppedPath   = "/sys/class/net/%s/statistics/rx_dropped"
	txDroppedPath   = "/sys/class/net/%s/statistics/tx_dropped"
	defaultInterval = 5
)

var (
	allStats             = true
	showCount            = false
	showDropped          = false
	showTransfer         = false
	onlyRX               = false
	onlyTX               = false
	hideNetworkInterface = false
	outputFreq           = defaultInterval
	interfaceName        string
	state                NetworkStats
)

func init() {
	flag.BoolVar(&allStats, "all", false, "Show all network link statistics ")
	flag.BoolVar(&showCount, "count", false, "Show statistics about packet count")
	flag.BoolVar(&showTransfer, "transfer", false, "Show statistics about total bytes transferred")
	flag.BoolVar(&showDropped, "dropped", false, "Show statistics about dropped packets")
	flag.BoolVar(&onlyRX, "only-rx", false, "Show only received packets statistics")
	flag.BoolVar(&onlyTX, "only-tx", false, "Show only sent packets statistics")
	flag.BoolVar(&hideNetworkInterface, "hideNetworkInterface", false, "Don't print the network interface name")
	flag.IntVar(&outputFreq, "output-frequency", defaultInterval, "Output frequency in seconds (output will be averaged to the interval)")
	flag.Usage = func() {
		fmt.Println("Usage: gnm [options] <network-interface>")
		fmt.Println("repository: https://github.com/rafa-dot-el/gonetmon")
		fmt.Println("Options:")
		flag.PrintDefaults()
	}
}

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) != 1 {
		flag.Usage()
		os.Exit(1)
	}
	interfaceName = args[0]

	// In case any other flag is set, automatically disable showing all stats
	if showCount || showTransfer || showDropped {
		allStats = false
	}

	// Setup signal handling for Ctrl+C
	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	// Start monitoring loop
	ticker := time.NewTicker(time.Duration(outputFreq) * time.Second)
	quit := make(chan bool)
	error := state.Read(interfaceName)
	if error != nil {
		fmt.Printf("Error reading interface %s data: %s", interfaceName, error)
	}
	go monitorNetworkStats(ticker, quit)

	// Wait for Ctrl+C
	<-signalChannel

	// Cleanup and exit
	ticker.Stop()
	quit <- true
	fmt.Println("Program terminated.")
}

func monitorNetworkStats(ticker *time.Ticker, quit chan bool) {
	for {
		select {
		case <-ticker.C:
			printNetworkStats()
		case <-quit:
			return
		}
	}
}

func convertUnit(value uint64, unit string) (float64, string) {
	divisors := []struct {
		divisor uint64
		unit    string
	}{
		{1000, "k"},
		{1000000, "m"},
		{1000000000, "g"},
		{1000000000000, "t"},
	}

	resultUnit := unit
	resultValue := float64(value)
	for _, divisor := range divisors {
		if value >= divisor.divisor {
			resultValue = float64(value) / float64(divisor.divisor)
			resultUnit = divisor.unit + unit
		} else {
			break
		}
	}

	return resultValue, resultUnit
}

func printNetworkStats() {

	// Print the statistics
	if !hideNetworkInterface {
		fmt.Printf("%s: ", interfaceName)
	}

	// Read network statistics from the specified files
	newState := NetworkStats{}

	error := newState.Read(interfaceName)
	if error != nil {
		fmt.Printf("Error reading interface %s data: %s", interfaceName, error)
		return
	}

	diff := state.NormalizeDiff(&newState, uint64(outputFreq))
	state = newState

	needComma := allStats
	if allStats || showCount {
		unit := "p/s"
		if onlyRX {
			packetsRX, unitRX := convertUnit(diff.packetsRX, unit)
			fmt.Printf("Packets %0.1f %s", packetsRX, unitRX)
		} else if onlyTX {
			packetsTX, unitTX := convertUnit(diff.packetsTX, unit)
			fmt.Printf("Packets %0.1f %s", packetsTX, unitTX)
		} else {
			packetsRX, unitRX := convertUnit(diff.packetsRX, unit)
			packetsTX, unitTX := convertUnit(diff.packetsTX, unit)
			fmt.Printf("Packets RX:%0.1f %s TX:%0.1f %s", packetsRX, unitRX, packetsTX, unitTX)
		}
		needComma = true
	}
	if allStats || showTransfer {
		if needComma {
			fmt.Print(",")
		}
		unit := "b/s"
		if onlyRX {
			bytesRX, unitRX := convertUnit(diff.bytesRX, unit)
			fmt.Printf("Data %0.1f %s", bytesRX, unitRX)
		} else if onlyTX {
			bytesTX, unitTX := convertUnit(diff.bytesTX, unit)
			fmt.Printf("Data %0.1f %s", bytesTX, unitTX)
		} else {
			bytesRX, unitRX := convertUnit(diff.bytesRX, unit)
			bytesTX, unitTX := convertUnit(diff.bytesTX, unit)
			fmt.Printf("Data RX:%0.1f %s TX:%0.1f %s", bytesRX, unitRX, bytesTX, unitTX)
		}
		needComma = true
	}

	if allStats || showDropped {
		if needComma {
			fmt.Print(",")
		}
		unit := "d/s"

		if onlyRX {
			droppedRX, unitRX := convertUnit(diff.droppedRX, unit)
			fmt.Printf("Drops %0.1f %s", droppedRX, unitRX)
		} else if onlyTX {
			droppedTX, unitTX := convertUnit(diff.droppedTX, unit)
			fmt.Printf("Drops %0.1f %s", droppedTX, unitTX)
		} else {
			droppedRX, unitRX := convertUnit(diff.droppedRX, unit)
			droppedTX, unitTX := convertUnit(diff.droppedTX, unit)
			fmt.Printf("Drops RX:%0.1f %s TX:%0.1f %s", droppedRX, unitRX, droppedTX, unitTX)
		}
	}
	fmt.Println()
}

func readUint64FromFile(filePath string) (uint64, error) {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return 0, fmt.Errorf("Error reading file %s: %v", filePath, err)
	}
	value, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("Error parsing value from %s: %v", filePath, err)
	}
	return value, nil
}

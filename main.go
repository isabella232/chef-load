package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/go-chef/chef"
	log "github.com/sirupsen/logrus"
)

// AppVersion - Application Version
const AppVersion = "2.2.0"

var config *chefLoadConfig

type UTCFormatter struct {
	log.Formatter
}

func (u UTCFormatter) Format(e *log.Entry) ([]byte, error) {
	e.Time = e.Time.UTC()
	return u.Formatter.Format(e)
}

var logger = log.New()

func init() {
	fConfig := flag.String("config", "", "Configuration file to load")
	fHelp := flag.Bool("help", false, "Print this help")
	fNodeNamePrefix := flag.String("prefix", "", "This prefix will go at the beginning of each node name")
	fNumNodes := flag.String("nodes", "", "The number of nodes to simulate")
	fInterval := flag.String("interval", "", "Interval between a node's chef-client runs, in minutes")
	fSampleConfig := flag.Bool("sample-config", false, "Print out full sample configuration")
	fVersion := flag.Bool("version", false, "Print chef-load version")
	flag.Parse()

	if *fHelp {
		fmt.Println("Usage of chef-load:")
		flag.PrintDefaults()
		os.Exit(0)
	}

	if *fVersion {
		fmt.Println("chef-load", AppVersion)
		os.Exit(0)
	}

	if *fSampleConfig {
		printSampleConfig()
		os.Exit(0)
	}

	if *fConfig != "" {
		var err error
		config, err = loadConfig(*fConfig)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		fmt.Println("Usage of chef-load:")
		flag.PrintDefaults()
		return
	}

	if *fNumNodes != "" {
		config.NumNodes, _ = strconv.Atoi(*fNumNodes)
	}

	if *fInterval != "" {
		config.Interval, _ = strconv.Atoi(*fInterval)
	}

	if *fNodeNamePrefix != "" {
		config.NodeNamePrefix = *fNodeNamePrefix
	}

	if config.ChefServerURL == "" && config.DataCollectorURL == "" {
		fmt.Println("ERROR: You must set chef_server_url or data_collector_url or both")
		os.Exit(1)
	}

	if config.ChefServerURL != "" {
		config.RunChefClient = true
		if config.ClientName == "" || config.ClientKey == "" {
			fmt.Println("ERROR: You must set client_name and client_key if chef_server_url is set")
			os.Exit(1)
		}
	}

	if config.DataCollectorURL != "" && config.ChefServerURL == "" {
		// make sure config.ChefServerURL is set to something because it is used
		// even when only in data-collector mode
		config.ChefServerURL = "https://chef.example.com/organizations/demo/"
	}

	if config.RunChefClient {
		// Early exit if we can't read the client_key, avoiding a messy stacktrace
		f, err := os.Open(config.ClientKey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Could not read client key %v:\n\t%v\n", config.ClientKey, err)
			os.Exit(1)
		}
		f.Close()
	}

	logger.Formatter = UTCFormatter{&log.JSONFormatter{}}

	if err := os.MkdirAll(path.Dir(config.LogFile), 0755); err != nil {
		log.WithField("error", err).Fatal("Failed to create directory")
	}
	file, err := os.OpenFile(config.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		logger.Out = file
	} else {
		log.WithField("error", err).Fatal("Failed to log to file")
	}
}

func main() {
	var nodeClient chef.Client
	if config.RunChefClient {
		nodeClient = getAPIClient(config.ClientName, config.ClientKey, config.ChefServerURL)
	}

	ohaiJSON := map[string]interface{}{}
	if config.OhaiJSONFile != "" {
		ohaiJSON = parseJSONFile(config.OhaiJSONFile)
	}

	convergeJSON := map[string]interface{}{}
	if config.ConvergeStatusJSONFile != "" {
		convergeJSON = parseJSONFile(config.ConvergeStatusJSONFile)
	}

	complianceJSON := map[string]interface{}{}
	if config.ComplianceStatusJSONFile != "" {
		complianceJSON = parseJSONFile(config.ComplianceStatusJSONFile)
	}

	delayBetweenNodes := time.Duration(math.Ceil(float64(time.Duration(config.Interval)*(time.Minute/time.Nanosecond))/float64(config.NumNodes))) * time.Nanosecond
	firstRun := true
	for {
		for i := 1; i <= config.NumNodes; i++ {
			nodeName := config.NodeNamePrefix + "-" + strconv.Itoa(i)
			go chefClientRun(nodeClient, nodeName, firstRun, ohaiJSON, convergeJSON, complianceJSON)
			time.Sleep(delayBetweenNodes)
		}
		firstRun = false
	}
}

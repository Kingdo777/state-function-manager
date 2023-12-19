package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"

	df "github.com/Kingdo777/state_function_manager/state_function"
)

// flag to show version
var version = flag.Bool("version", false, "show version")

// flag to enable debug
var debug = flag.Bool("debug", false, "enable debug output")

// flag to pass an environment as a json string
var env = flag.String("env", "", "pass an environment as a json string")

func main() {
	flag.Parse()

	// show version number
	if *version {
		fmt.Printf("StateFunction ManagerLoop Proxy v%s, built with %s\n", df.Version, runtime.Version())
		return
	}

	// debugging
	if *debug {
		// set debugging flag, propagated to the actions
		df.Debugging = true
		err := os.Setenv("DF_DEBUG", "1")
		if err != nil {
			log.Fatal(err)
			return
		}
	}

	// show user defined env info
	if *env != "" {
		df.Debug(*env)

	}

	//// create the manager proxy
	mp := df.NewManagerProxy()

	// start the balls rolling
	df.Debug("OpenWhisk ActionLoop Proxy %s: starting", df.Version)
	mp.Start(7070)

}

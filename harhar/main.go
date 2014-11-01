// This command will do a GET request on a provided list of URLs, optionally
// logging them to the HAR file. It's a simple example that concisely showcases
// all the features and usage.

package main

import (
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/stridatum/harhar"
)

var (
	input  = flag.String("urls", "", "input urls (one per line)")
	output = flag.String("har", "", "output har to file")

	// client uses the interface just to show how it works.
	// typically you'd use this so that you can toggle logging on an off
	// at will to conserve memory usage.
	client harhar.ClientInterface = &http.Client{}
)

func main() {
	flag.Parse()
	if *input == "" {
		flag.Usage()
		os.Exit(1)
	}

	if *output == "" {
		log.Println("-har not provided, no .har file will be produced")
	} else {
		// wrap the http.Client to transparently track requests
		client = harhar.NewClient(client.(*http.Client))
	}

	////////

	// read in a file consisting of 1 line per URL, and do a GET on each.
	data, err := ioutil.ReadFile(*input)
	if err != nil {
		log.Println("error reading input: ", err)
		os.Exit(1)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		resp, err := client.Get(line)
		if err != nil {
			log.Println("error in GET to", line)
		} else {
			log.Printf("Got %s from %s\n", resp.Status, line)
		}
	}

	///////////

	if *output != "" {
		size, err := client.(*harhar.Client).WriteLog(*output)
		if err == nil {
			// it's always good to report size when logging since memory usage
			// will grow pretty quickly if you're not careful.
			log.Printf("wrote %s (%.1fkb)\n", *output, float64(size)/1024.0)
		} else {
			log.Println("error writing har: ", err)
		}
	}
}

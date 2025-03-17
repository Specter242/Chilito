package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/yourusername/chilito/finder"
)

func main() {
	var address string
	var radius int
	var verbose bool
	var debugDelay int

	flag.StringVar(&address, "address", "", "Address to search from (required)")
	flag.IntVar(&radius, "radius", 100000, "Search radius in meters (default 100km)")
	flag.BoolVar(&verbose, "verbose", false, "Enable verbose output")
	flag.IntVar(&debugDelay, "delay", 0, "Add delay between API calls in seconds (for debugging)")
	flag.Parse()

	if address == "" {
		flag.Usage()
		return
	}

	// Set up logging based on verbosity
	if verbose {
		fmt.Println("Verbose mode enabled")
	} else {
		// Reduce log output in normal mode
		log.SetOutput(os.Stderr)
	}

	fmt.Printf("Searching for Chili Cheese Burrito near: %s (within %d meters)\n", address, radius)

	// Create the finder (simplified to remove OAuth and API key options)
	chilitoFinder := finder.NewChilitoBurritoFinder()

	// If debug delay is set, display a message
	if debugDelay > 0 {
		fmt.Printf("Debug delay is set to %d seconds between API calls\n", debugDelay)
	}

	startTime := time.Now()
	result, err := chilitoFinder.FindNearestChilitoBurrito(address, radius)
	searchDuration := time.Since(startTime)

	if err != nil {
		log.Fatalf("Error finding Chilito burrito: %v", err)
	}

	fmt.Printf("\nSearch completed in %v\n", searchDuration.Round(time.Second))

	if result != nil {
		fmt.Printf("\nSUCCESS! Found Chilito Burrito at: %s\n", result.Name)
		fmt.Printf("Address: %s\n", result.Address)
		fmt.Printf("Distance: %.2f km\n", result.Distance)
		fmt.Printf("Phone: %s\n", result.PhoneNumber)
	} else {
		fmt.Println("\nNo Taco Bell locations with Chilito Burrito found within the search radius.")
		fmt.Println("Try increasing the search radius or using a different starting address.")
	}
}

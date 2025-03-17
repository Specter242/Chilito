package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/yourusername/chilito/finder"
)

func main() {
	var address string
	var radius int
	var verbose bool
	var debugDelay int
	var useOAuth bool
	var credentialsPath string
	const apiKey = "AIzaSyASju5Gu_8bId7mXVzV-zflf2vxJN5LqhU"

	// Default credentials path
	defaultCredentialsPath := filepath.Join(os.Getenv("USERPROFILE"), "Downloads",
		"client_secret_150000384307-ndkt6ot3smdl37pdj1vqk84hffgfk9bg.apps.googleusercontent.com.json")

	flag.StringVar(&address, "address", "", "Address to search from (required)")
	flag.IntVar(&radius, "radius", 100000, "Search radius in meters (default 100km)")
	flag.BoolVar(&verbose, "verbose", false, "Enable verbose output")
	flag.IntVar(&debugDelay, "delay", 0, "Add delay between API calls in seconds (for debugging)")
	flag.BoolVar(&useOAuth, "oauth", false, "Use OAuth authentication instead of API key")
	flag.StringVar(&credentialsPath, "credentials", defaultCredentialsPath, "Path to OAuth credentials JSON file")
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

	// Create the finder with either API key or OAuth
	var chilitoFinder *finder.ChilitoBurritoFinder
	var err error

	if useOAuth {
		fmt.Printf("Using OAuth authentication with credentials from: %s\n", credentialsPath)
		chilitoFinder, err = finder.NewChilitoBurritoFinderWithOAuth(credentialsPath)
		if err != nil {
			log.Fatalf("Failed to create OAuth authenticated finder: %v", err)
		}
	} else {
		fmt.Println("Using API key authentication")
		chilitoFinder = finder.NewChilitoBurritoFinder(apiKey)
	}

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

package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/yourusername/chilito/finder"
)

func main() {
	var address string
	var radius int
	const apiKey = "AIzaSyAiMObdV5w-zFahCX-8kelmElJqkSgEWjU"

	flag.StringVar(&address, "address", "", "Address to search from (required)")
	flag.IntVar(&radius, "radius", 50000, "Search radius in meters (default 50km)")
	flag.Parse()

	if address == "" {
		flag.Usage()
		return
	}

	chilitoFinder := finder.NewChilitoBurritoFinder(apiKey)

	result, err := chilitoFinder.FindNearestChilitoBurrito(address, radius)
	if err != nil {
		log.Fatalf("Error finding Chilito burrito: %v", err)
	}

	if result != nil {
		fmt.Printf("Found Chilito Burrito at: %s\n", result.Name)
		fmt.Printf("Address: %s\n", result.Address)
		fmt.Printf("Distance: %.2f km\n", result.Distance)
		fmt.Printf("Phone: %s\n", result.PhoneNumber)
	} else {
		fmt.Println("No Taco Bell locations with Chilito Burrito found within the search radius.")
	}
}

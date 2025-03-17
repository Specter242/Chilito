package main

import (
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/yourusername/chilito/finder" // Updated to match main.go's import
)

// SearchMenuForChilito checks if a location has the Chili Cheese Burrito
func SearchMenuForChilito(location finder.TacoBellLocation) (bool, error) {
	log.Printf("Checking menu at Taco Bell %s (%s)...", location.StoreID, location.Name)

	// Only use web scraping to check for the Chilito
	return checkWithWebScraping(location)
}

// checkWithWebScraping uses web scraping to check the menu
func checkWithWebScraping(location finder.TacoBellLocation) (bool, error) {
	// List of URLs to check
	urls := []string{
		fmt.Sprintf("https://www.tacobell.com/food/menu?store=%s", location.StoreID),
		fmt.Sprintf("https://www.tacobell.com/food/burritos?store=%s", location.StoreID),
		fmt.Sprintf("https://www.tacobell.com/food/specialty?store=%s", location.StoreID),
	}

	// Check each URL
	for _, url := range urls {
		found, err := checkURL(url)
		if err != nil {
			log.Printf("Failed to access %s after multiple attempts", url)
			continue
		}
		if found {
			log.Printf("Found 'chili cheese burrito' in menu at %s!", url)
			return true, nil
		}
	}

	return false, nil
}

// checkURL checks a specific URL for mentions of the Chili Cheese Burrito
func checkURL(url string) (bool, error) {
	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		// Add delay between retries
		if attempt > 0 {
			time.Sleep(2 * time.Second)
		}

		// Make the HTTP request
		client := &http.Client{
			Timeout: 10 * time.Second,
		}
		resp, err := client.Get(url)
		if err != nil {
			log.Printf("Error accessing %s (attempt %d): %v", url, attempt+1, err)
			continue
		}

		// Check response status
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			log.Printf("Received status code %d for %s (attempt %d)", resp.StatusCode, url, attempt+1)
			continue
		}

		// Parse the HTML document
		doc, err := goquery.NewDocumentFromReader(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("Error parsing HTML for %s (attempt %d): %v", url, attempt+1, err)
			continue
		}

		// Get the HTML content
		html, err := doc.Html()
		if err != nil {
			log.Printf("Error getting HTML content for %s (attempt %d): %v", url, attempt+1, err)
			continue
		}

		// Check for Chili Cheese Burrito in the HTML (case insensitive)
		htmlLower := strings.ToLower(html)
		keywords := []string{"chili cheese burrito", "chilito", "chili burrito", "cheesy beefy melt"}
		for _, keyword := range keywords {
			if strings.Contains(htmlLower, keyword) {
				return true, nil
			}
		}

		// Additional check for product names in specific elements
		found := false
		doc.Find(".product-name").Each(func(i int, s *goquery.Selection) {
			productName := strings.ToLower(s.Text())
			for _, keyword := range keywords {
				if strings.Contains(productName, keyword) {
					found = true
				}
			}
		})

		if found {
			return true, nil
		}

		// Also check with regex for more complex patterns
		chilitoPattern := regexp.MustCompile(`(?i)chil(i|ito).*burrito|burrito.*chil(i|ito)`)
		if chilitoPattern.MatchString(html) {
			return true, nil
		}

		// If we got here, we didn't find it in this URL
		return false, nil
	}

	// If we've tried multiple times and still failed, return error
	return false, fmt.Errorf("failed to access URL after %d attempts", maxRetries)
}

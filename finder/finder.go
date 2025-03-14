package finder

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// TacoBellLocation represents a Taco Bell restaurant
type TacoBellLocation struct {
	PlaceID     string
	Name        string
	Address     string
	Distance    float64 // in kilometers
	PhoneNumber string
	StoreID     string
}

// ChilitoBurritoFinder manages searching for the Chilito Burrito
type ChilitoBurritoFinder struct {
	apiKey string
}

// NewChilitoBurritoFinder creates a new finder instance
func NewChilitoBurritoFinder(apiKey string) *ChilitoBurritoFinder {
	return &ChilitoBurritoFinder{
		apiKey: apiKey,
	}
}

// FindNearestChilitoBurrito finds the nearest Taco Bell with a Chili Cheese Burrito
func (f *ChilitoBurritoFinder) FindNearestChilitoBurrito(address string, radius int) (*TacoBellLocation, error) {
	// Get coordinates for the address
	lat, lng, err := f.geocodeAddress(address)
	if err != nil {
		return nil, fmt.Errorf("geocoding error: %w", err)
	}

	// Find Taco Bell locations near these coordinates
	locations, err := f.findTacoBellLocations(lat, lng, radius)
	if err != nil {
		return nil, fmt.Errorf("location search error: %w", err)
	}

	if len(locations) == 0 {
		return nil, errors.New("no Taco Bell locations found in the specified radius")
	}

	// Sort locations by distance
	sort.Slice(locations, func(i, j int) bool {
		return locations[i].Distance < locations[j].Distance
	})

	// Check each location for the Chilito/Chili Cheese Burrito
	for _, location := range locations {
		fmt.Printf("Checking menu at %s (%.2f km away)...\n", location.Name, location.Distance)

		// Get the store ID from the Taco Bell website
		storeID, err := f.getStoreID(location)
		if err != nil {
			fmt.Printf("Error getting store ID for %s: %v\n", location.Name, err)
			continue
		}
		location.StoreID = storeID

		// Check if this store has the Chilito
		hasChilito, err := f.checkForChilitoBurrito(location)
		if err != nil {
			fmt.Printf("Error checking menu at %s: %v\n", location.Name, err)
			continue
		}

		if hasChilito {
			return &location, nil
		}

		fmt.Printf("Chilito Burrito not found at %s\n", location.Name)
	}

	return nil, nil
}

// geocodeAddress converts an address to coordinates
func (f *ChilitoBurritoFinder) geocodeAddress(address string) (float64, float64, error) {
	endpoint := "https://maps.googleapis.com/maps/api/geocode/json"

	params := url.Values{}
	params.Add("address", address)
	params.Add("key", f.apiKey)

	resp, err := http.Get(endpoint + "?" + params.Encode())
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()

	var result struct {
		Results []struct {
			Geometry struct {
				Location struct {
					Lat float64 `json:"lat"`
					Lng float64 `json:"lng"`
				} `json:"location"`
			} `json:"geometry"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, 0, err
	}

	if len(result.Results) == 0 {
		return 0, 0, errors.New("no results found for the address")
	}

	return result.Results[0].Geometry.Location.Lat, result.Results[0].Geometry.Location.Lng, nil
}

// findTacoBellLocations finds Taco Bell restaurants near coordinates
func (f *ChilitoBurritoFinder) findTacoBellLocations(lat, lng float64, radius int) ([]TacoBellLocation, error) {
	endpoint := "https://maps.googleapis.com/maps/api/place/nearbysearch/json"

	params := url.Values{}
	params.Add("location", fmt.Sprintf("%f,%f", lat, lng))
	params.Add("radius", strconv.Itoa(radius))
	params.Add("keyword", "Taco Bell")
	params.Add("type", "restaurant")
	params.Add("key", f.apiKey)

	var locations []TacoBellLocation
	var pagetoken string

	for {
		requestURL := endpoint + "?" + params.Encode()
		if pagetoken != "" {
			requestURL += "&pagetoken=" + pagetoken
		}

		resp, err := http.Get(requestURL)
		if err != nil {
			return nil, err
		}

		var result struct {
			Results []struct {
				PlaceID  string `json:"place_id"`
				Name     string `json:"name"`
				Vicinity string `json:"vicinity"` // address
				Geometry struct {
					Location struct {
						Lat float64 `json:"lat"`
						Lng float64 `json:"lng"`
					} `json:"location"`
				} `json:"geometry"`
			} `json:"results"`
			NextPageToken string `json:"next_page_token"`
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err := json.Unmarshal(body, &result); err != nil {
			return nil, err
		}

		for _, place := range result.Results {
			if strings.Contains(strings.ToLower(place.Name), "taco bell") {
				// Calculate distance from origin to this location
				distance := haversineDistance(lat, lng,
					place.Geometry.Location.Lat,
					place.Geometry.Location.Lng)

				// Get additional details like phone number
				details, err := f.getPlaceDetails(place.PlaceID)
				if err != nil {
					fmt.Printf("Warning: couldn't get details for %s: %v\n", place.Name, err)
				}

				locations = append(locations, TacoBellLocation{
					PlaceID:     place.PlaceID,
					Name:        place.Name,
					Address:     place.Vicinity,
					Distance:    distance,
					PhoneNumber: details.PhoneNumber,
				})
			}
		}

		pagetoken = result.NextPageToken
		if pagetoken == "" {
			break
		}

		// Need to wait a bit before using the page token
		// We could implement a short delay here if needed
	}

	return locations, nil
}

type placeDetails struct {
	PhoneNumber string
}

// getPlaceDetails gets additional details for a place
func (f *ChilitoBurritoFinder) getPlaceDetails(placeID string) (placeDetails, error) {
	endpoint := "https://maps.googleapis.com/maps/api/place/details/json"

	params := url.Values{}
	params.Add("place_id", placeID)
	params.Add("fields", "formatted_phone_number")
	params.Add("key", f.apiKey)

	resp, err := http.Get(endpoint + "?" + params.Encode())
	if err != nil {
		return placeDetails{}, err
	}
	defer resp.Body.Close()

	var result struct {
		Result struct {
			FormattedPhoneNumber string `json:"formatted_phone_number"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return placeDetails{}, err
	}

	return placeDetails{
		PhoneNumber: result.Result.FormattedPhoneNumber,
	}, nil
}

// getStoreID gets the Taco Bell store ID which is needed for menu checking
func (f *ChilitoBurritoFinder) getStoreID(location TacoBellLocation) (string, error) {
	// Format the address for URL query
	formattedAddress := url.QueryEscape(location.Address)
	locationURL := fmt.Sprintf("https://www.tacobell.com/locations/search?q=%s", formattedAddress)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	// Create a request with headers to mimic a browser
	req, err := http.NewRequest("GET", locationURL, nil)
	if err != nil {
		return "", err
	}

	// Set common headers to avoid being blocked
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/96.0.4664.110 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error accessing Taco Bell location search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("received non-200 response: %d", resp.StatusCode)
	}

	// Parse HTML
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error parsing HTML: %w", err)
	}

	// Look for store ID in several possible locations
	var storeID string

	// First approach: Look for data attributes in location cards
	doc.Find(".location-card, .store-card, [data-store-id]").Each(func(i int, s *goquery.Selection) {
		if id, exists := s.Attr("data-store-id"); exists && storeID == "" {
			// Check if address matches approximately
			cardAddress := s.Find(".address, .location-address").Text()
			if similarAddresses(cardAddress, location.Address) {
				storeID = id
			}
		}
	})

	// Second approach: Look for store ID in script tags
	if storeID == "" {
		doc.Find("script").Each(func(i int, s *goquery.Selection) {
			script := s.Text()
			if strings.Contains(script, "storeId") || strings.Contains(script, "store_id") {
				// Use regex to find store ID
				re := regexp.MustCompile(`(?:storeId|store_id)[\s:"'=]+(\d+)`)
				matches := re.FindStringSubmatch(script)
				if len(matches) >= 2 {
					storeID = matches[1]
				}
			}
		})
	}

	// Third approach: Look for it in URLs on the page
	if storeID == "" {
		doc.Find("a[href*='store='], a[href*='storeId=']").Each(func(i int, s *goquery.Selection) {
			href, exists := s.Attr("href")
			if !exists {
				return
			}

			// Extract store ID from URL
			re := regexp.MustCompile(`(?:store|storeId)=(\d+)`)
			matches := re.FindStringSubmatch(href)
			if len(matches) >= 2 {
				storeID = matches[1]
			}
		})
	}

	// If we still don't have a store ID, use the Place ID as a fallback
	if storeID == "" {
		fmt.Printf("Warning: Could not find store ID for %s, using fallback\n", location.Name)
		storeID = location.PlaceID
	}

	return storeID, nil
}

// similarAddresses checks if two addresses are likely the same location
func similarAddresses(addr1, addr2 string) bool {
	// Normalize addresses: remove punctuation, extra spaces, and convert to lowercase
	normalize := func(s string) string {
		s = strings.ToLower(s)
		s = regexp.MustCompile(`[^\w\s]`).ReplaceAllString(s, " ")
		s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
		return strings.TrimSpace(s)
	}

	addr1Norm := normalize(addr1)
	addr2Norm := normalize(addr2)

	// If one is contained in the other, consider them similar
	if strings.Contains(addr1Norm, addr2Norm) || strings.Contains(addr2Norm, addr1Norm) {
		return true
	}

	// Compare important parts (street number, name, city)
	words1 := strings.Fields(addr1Norm)
	words2 := strings.Fields(addr2Norm)

	matches := 0
	totalWords := math.Max(float64(len(words1)), float64(len(words2)))

	for _, w1 := range words1 {
		if len(w1) < 2 {
			continue // Skip very short words
		}
		for _, w2 := range words2 {
			if w1 == w2 || (len(w1) > 4 && strings.Contains(w2, w1)) {
				matches++
				break
			}
		}
	}

	// If at least 60% of words match, consider them similar
	return float64(matches)/totalWords > 0.6
}

// checkForChilitoBurrito checks if a location has the Chili Cheese Burrito
func (f *ChilitoBurritoFinder) checkForChilitoBurrito(location TacoBellLocation) (bool, error) {
	// URLs to check for menu
	urls := []string{
		fmt.Sprintf("https://www.tacobell.com/food/menu?store=%s", location.StoreID),
		fmt.Sprintf("https://www.tacobell.com/food/burritos?store=%s", location.StoreID),
		fmt.Sprintf("https://www.tacobell.com/food/specialties?store=%s", location.StoreID),
		// URL for the "Chili Cheese" page if it exists
		fmt.Sprintf("https://www.tacobell.com/food/specialty/chili-cheese?store=%s", location.StoreID),
	}

	// Terms that indicate the Chilito/Chili Cheese Burrito
	searchTerms := []string{
		"chili cheese burrito",
		"chilito",
		"chili burrito",
		"ccb",
		"chili cheese wrap",
	}

	// Create client
	client := &http.Client{
		Timeout: 20 * time.Second,
	}

	// Try each URL
	for _, menuURL := range urls {
		// Try up to 3 times per URL
		var resp *http.Response
		var err error

		for attempt := 0; attempt < 3; attempt++ {
			req, err := http.NewRequest("GET", menuURL, nil)
			if err != nil {
				continue
			}

			// Set headers to appear like a normal browser
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/96.0.4664.110 Safari/537.36")
			req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")

			resp, err = client.Do(req)
			if err == nil && resp.StatusCode == http.StatusOK {
				break
			}

			if resp != nil {
				resp.Body.Close()
			}

			// Wait before retrying
			time.Sleep(time.Duration(attempt+1) * time.Second)
		}

		if err != nil || resp == nil {
			fmt.Printf("Failed to access %s after multiple attempts\n", menuURL)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			fmt.Printf("Received status %d for %s\n", resp.StatusCode, menuURL)
			continue
		}

		// Parse HTML
		doc, err := goquery.NewDocumentFromReader(resp.Body)
		resp.Body.Close()

		if err != nil {
			fmt.Printf("Error parsing HTML: %v\n", err)
			continue
		}

		// Look for menu items with the target terms
		found := false

		// Check specific menu item selectors
		selectors := []string{
			".menu-item", ".item-tile", ".product-name", ".product-title",
			".food-item", ".item-name", ".menu-product", ".product-card",
		}

		for _, selector := range selectors {
			doc.Find(selector).Each(func(i int, s *goquery.Selection) {
				itemText := strings.ToLower(s.Text())
				for _, term := range searchTerms {
					if strings.Contains(itemText, term) {
						found = true
						return
					}
				}
			})

			if found {
				break
			}
		}

		// If found in specific selectors, return true
		if found {
			return true, nil
		}

		// As a fallback, check the entire page content
		pageContent := strings.ToLower(doc.Text())
		for _, term := range searchTerms {
			if strings.Contains(pageContent, term) {
				// Look at surrounding text to confirm it's a menu item
				re := regexp.MustCompile(fmt.Sprintf(`.{0,50}%s.{0,50}`, regexp.QuoteMeta(term)))
				matches := re.FindAllString(pageContent, -1)

				for _, match := range matches {
					// If the surrounding text suggests it's a menu item (has price, description, etc.)
					if strings.Contains(match, "price") ||
						strings.Contains(match, "$") ||
						strings.Contains(match, "order") ||
						strings.Contains(match, "menu") {
						return true, nil
					}
				}
			}
		}
	}

	// Alternative approach: check the "Chilito Finder" website (if it exists)
	// This is a hypothetical site that might track Chili Cheese Burrito availability
	resp, err := http.Get(fmt.Sprintf("https://chilicheeseburrito.com/locations?id=%s", location.StoreID))
	if err == nil && resp.StatusCode == http.StatusOK {
		doc, err := goquery.NewDocumentFromReader(resp.Body)
		resp.Body.Close()

		if err == nil {
			// Look for indications this location has the Chilito
			available := strings.Contains(strings.ToLower(doc.Text()), "available")
			if available {
				return true, nil
			}
		}
	}

	return false, nil
}

// haversineDistance calculates the distance between two points in kilometers
func haversineDistance(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadius = 6371.0 // kilometers

	// Convert latitude and longitude from degrees to radians
	lat1 = lat1 * (3.14159265359 / 180.0)
	lng1 = lng1 * (3.14159265359 / 180.0)
	lat2 = lat2 * (3.14159265359 / 180.0)
	lng2 = lng2 * (3.14159265359 / 180.0)

	dlat := lat2 - lat1
	dlng := lng2 - lng1

	a := (1-0.5*dlat*dlat)*(1-0.5*dlng*dlng*0.5*(1-0.25*dlat*dlat)) - 0.5*dlat*dlat
	c := 2 * 0.5 * (1.5707963267948966 - a)
	d := earthRadius * c

	return d
}

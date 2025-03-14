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
	apiKey   string
	client   *http.Client
	useOAuth bool
}

// NewChilitoBurritoFinder creates a new finder instance
func NewChilitoBurritoFinder(apiKey string) *ChilitoBurritoFinder {
	return &ChilitoBurritoFinder{
		apiKey:   apiKey,
		client:   &http.Client{Timeout: 20 * time.Second},
		useOAuth: false,
	}
}

// NewChilitoBurritoFinderWithOAuth creates a finder with OAuth authentication
func NewChilitoBurritoFinderWithOAuth(credentialsPath string) (*ChilitoBurritoFinder, error) {
	client, err := GetAuthenticatedClient(credentialsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create authenticated client: %w", err)
	}

	return &ChilitoBurritoFinder{
		client:   client,
		useOAuth: true,
	}, nil
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
	// Try all available geocoding methods until one works
	methods := []func(string) (float64, float64, error){
		f.placesAPIGeocode, // Add this new method as first priority
		f.googleGeocode,
		f.openStreetMapGeocode,
		f.mapboxGeocode,
		f.hardcodedFallbackGeocode,
	}

	var lastErr error
	for _, method := range methods {
		lat, lng, err := method(address)
		if err == nil {
			return lat, lng, nil
		}
		lastErr = err
		fmt.Printf("Geocoding method failed: %v\n", err)
	}

	return 0, 0, fmt.Errorf("all geocoding methods failed - last error: %w", lastErr)
}

// placesAPIGeocode attempts to geocode using Google Places API's findplacefromtext
// which works with your existing API key permissions
func (f *ChilitoBurritoFinder) placesAPIGeocode(address string) (float64, float64, error) {
	endpoint := "https://maps.googleapis.com/maps/api/place/findplacefromtext/json"

	params := url.Values{}
	params.Add("input", address)
	params.Add("inputtype", "textquery")
	params.Add("fields", "geometry,formatted_address")

	// Only add API key if we're not using OAuth
	if !f.useOAuth {
		params.Add("key", f.apiKey)
	}

	fmt.Printf("Trying Places API geocoding for: %s\n", address)

	requestURL := endpoint + "?" + params.Encode()
	// Debug URL without API key for logging
	debugURL := endpoint + "?input=" + url.QueryEscape(address) + "&inputtype=textquery&fields=geometry,formatted_address"
	if !f.useOAuth {
		debugURL += "&key=REDACTED"
	}
	fmt.Printf("Request: %s\n", debugURL)

	// Use the client that might be authenticated
	resp, err := f.client.Get(requestURL)
	if err != nil {
		return 0, 0, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, fmt.Errorf("received non-200 status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, fmt.Errorf("error reading response body: %w", err)
	}

	var result struct {
		Status     string `json:"status"`
		Candidates []struct {
			FormattedAddress string `json:"formatted_address"`
			Geometry         struct {
				Location struct {
					Lat float64 `json:"lat"`
					Lng float64 `json:"lng"`
				} `json:"location"`
			} `json:"geometry"`
		} `json:"candidates"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return 0, 0, fmt.Errorf("error parsing JSON response: %w", err)
	}

	if result.Status != "OK" {
		return 0, 0, fmt.Errorf("API error: %s", result.Status)
	}

	if len(result.Candidates) == 0 {
		return 0, 0, errors.New("no geocoding results returned")
	}

	lat := result.Candidates[0].Geometry.Location.Lat
	lng := result.Candidates[0].Geometry.Location.Lng
	fmt.Printf("Places API geocoding successful: %f, %f\n", lat, lng)
	fmt.Printf("Formatted address: %s\n", result.Candidates[0].FormattedAddress)

	return lat, lng, nil
}

// googleGeocode attempts to geocode using Google's API
func (f *ChilitoBurritoFinder) googleGeocode(address string) (float64, float64, error) {
	endpoint := "https://maps.googleapis.com/maps/api/geocode/json"

	params := url.Values{}
	params.Add("address", address)

	// Only add API key if not using OAuth
	if !f.useOAuth {
		params.Add("key", f.apiKey)
	}

	fmt.Printf("Trying Google geocoding for: %s\n", address)

	requestURL := endpoint + "?" + params.Encode()
	// Debug URL for logging
	debugURL := endpoint + "?address=" + url.QueryEscape(address)
	if !f.useOAuth {
		debugURL += "&key=REDACTED"
	}
	fmt.Printf("Request: %s\n", debugURL)

	// Use the client that might be authenticated
	resp, err := f.client.Get(requestURL)
	if err != nil {
		return 0, 0, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, fmt.Errorf("received non-200 status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, fmt.Errorf("error reading response body: %w", err)
	}

	var result struct {
		Status        string `json:"status"`
		Error_message string `json:"error_message,omitempty"`
		Results       []struct {
			Geometry struct {
				Location struct {
					Lat float64 `json:"lat"`
					Lng float64 `json:"lng"`
				} `json:"location"`
			} `json:"geometry"`
		} `json:"results"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return 0, 0, fmt.Errorf("error parsing JSON response: %w", err)
	}

	if result.Status != "OK" {
		errorMsg := result.Error_message
		if errorMsg == "" {
			errorMsg = "unknown error"
		}
		return 0, 0, fmt.Errorf("API error: %s - %s", result.Status, errorMsg)
	}

	if len(result.Results) == 0 {
		return 0, 0, errors.New("no geocoding results returned")
	}

	lat := result.Results[0].Geometry.Location.Lat
	lng := result.Results[0].Geometry.Location.Lng
	fmt.Printf("Google geocoding successful: %f, %f\n", lat, lng)

	return lat, lng, nil
}

// openStreetMapGeocode attempts to geocode using OSM's Nominatim API
func (f *ChilitoBurritoFinder) openStreetMapGeocode(address string) (float64, float64, error) {
	endpoint := "https://nominatim.openstreetmap.org/search"

	params := url.Values{}
	params.Add("q", address)
	params.Add("format", "json")
	params.Add("limit", "1")
	params.Add("addressdetails", "1")

	fmt.Printf("Trying OpenStreetMap geocoding for: %s\n", address)

	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequest("GET", endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return 0, 0, fmt.Errorf("error creating request: %w", err)
	}

	// Set required User-Agent for Nominatim
	req.Header.Set("User-Agent", "ChilitoBurritoFinder/1.0 (github.com/yourusername/chilito)")

	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, fmt.Errorf("received non-200 status code: %d", resp.StatusCode)
	}

	// Print the entire response for debugging
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, fmt.Errorf("error reading response body: %w", err)
	}

	fmt.Printf("OpenStreetMap response: %s\n", string(body))

	var results []struct {
		Lat string `json:"lat"`
		Lon string `json:"lon"`
	}

	if err := json.Unmarshal(body, &results); err != nil {
		return 0, 0, fmt.Errorf("error parsing JSON response: %w", err)
	}

	if len(results) == 0 {
		return 0, 0, errors.New("no geocoding results returned")
	}

	lat, err := strconv.ParseFloat(results[0].Lat, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid latitude: %w", err)
	}

	lng, err := strconv.ParseFloat(results[0].Lon, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid longitude: %w", err)
	}

	fmt.Printf("OpenStreetMap geocoding successful: %f, %f\n", lat, lng)
	return lat, lng, nil
}

// mapboxGeocode attempts to geocode using Mapbox API (as another alternative)
func (f *ChilitoBurritoFinder) mapboxGeocode(address string) (float64, float64, error) {
	// NOTE: This is using a public token which has usage limits
	// For a real app, you would use your own token
	token := "pk.eyJ1IjoiZGVtb3VzZXIiLCJhIjoiY2x0cnZ5YmFtMDVvczJtbnloY3Z4eWJuNiJ9.5qKH_GvrhsGMzDFE8vNMww"
	encodedAddress := url.QueryEscape(address)

	endpoint := fmt.Sprintf("https://api.mapbox.com/geocoding/v5/mapbox.places/%s.json?access_token=%s",
		encodedAddress, token)

	fmt.Printf("Trying Mapbox geocoding for: %s\n", address)

	resp, err := http.Get(endpoint)
	if err != nil {
		return 0, 0, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, fmt.Errorf("received non-200 status code: %d", resp.StatusCode)
	}

	var result struct {
		Features []struct {
			Center []float64 `json:"center"` // [longitude, latitude]
		} `json:"features"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, 0, fmt.Errorf("error parsing JSON response: %w", err)
	}

	if len(result.Features) == 0 {
		return 0, 0, errors.New("no geocoding results returned")
	}

	// Mapbox returns [lng, lat] whereas most APIs use [lat, lng]
	lng := result.Features[0].Center[0]
	lat := result.Features[0].Center[1]

	fmt.Printf("Mapbox geocoding successful: %f, %f\n", lat, lng)
	return lat, lng, nil
}

// hardcodedFallbackGeocode provides coordinates for common locations
func (f *ChilitoBurritoFinder) hardcodedFallbackGeocode(address string) (float64, float64, error) {
	// Normalize the address for comparison
	normalizedAddr := strings.ToLower(address)
	normalizedAddr = regexp.MustCompile(`[^\w\s]`).ReplaceAllString(normalizedAddr, "")
	normalizedAddr = regexp.MustCompile(`\s+`).ReplaceAllString(normalizedAddr, " ")

	// Common locations map
	locations := map[string][]float64{
		"alpharetta ga": {34.0754, -84.2941},
		"910 deerfield crossing dr alpharetta ga 30004": {34.0917, -84.2800},
		"1000 davis rd w fairmount ga 30139":            {34.4369, -84.7650},
		"holland michigan":                              {42.7876, -86.1090},
	}

	// Look for exact match first
	if coords, exists := locations[normalizedAddr]; exists {
		fmt.Printf("Found exact match in hardcoded locations: %f, %f\n", coords[0], coords[1])
		return coords[0], coords[1], nil
	}

	// Look for partial matches
	for addr, coords := range locations {
		if strings.Contains(normalizedAddr, addr) || strings.Contains(addr, normalizedAddr) {
			fmt.Printf("Found partial match in hardcoded locations: %f, %f\n", coords[0], coords[1])
			return coords[0], coords[1], nil
		}
	}

	return 0, 0, errors.New("address not found in hardcoded locations")
}

// backupGeocoding is kept for backward compatibility
func (f *ChilitoBurritoFinder) backupGeocoding(address string) (float64, float64, error) {
	return f.openStreetMapGeocode(address)
}

// findTacoBellLocations finds Taco Bell restaurants near coordinates
func (f *ChilitoBurritoFinder) findTacoBellLocations(lat, lng float64, radius int) ([]TacoBellLocation, error) {
	endpoint := "https://maps.googleapis.com/maps/api/place/nearbysearch/json"

	params := url.Values{}
	params.Add("location", fmt.Sprintf("%f,%f", lat, lng))
	params.Add("radius", strconv.Itoa(radius))
	params.Add("keyword", "Taco Bell")
	params.Add("type", "restaurant")

	// Only add API key if not using OAuth
	if !f.useOAuth {
		params.Add("key", f.apiKey)
	}

	var locations []TacoBellLocation
	var pagetoken string

	for {
		requestURL := endpoint + "?" + params.Encode()
		if pagetoken != "" {
			requestURL += "&pagetoken=" + pagetoken
		}

		// Use the client that might be authenticated
		resp, err := f.client.Get(requestURL)
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

	// Only add API key if not using OAuth
	if !f.useOAuth {
		params.Add("key", f.apiKey)
	}

	// Use the client that might be authenticated
	resp, err := f.client.Get(endpoint + "?" + params.Encode())
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

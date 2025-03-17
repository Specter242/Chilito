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

// PlaceDetails stores additional details about a place
type PlaceDetails struct {
	PhoneNumber string
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
	// This method now returns an error since we've removed hardcoded locations
	return 0, 0, errors.New("no hardcoded locations available")
}

// findTacoBellLocations finds Taco Bell restaurants near coordinates
func (f *ChilitoBurritoFinder) findTacoBellLocations(lat, lng float64, radius int) ([]TacoBellLocation, error) {
	fmt.Printf("Searching for Taco Bell locations near coordinates: %f, %f (radius: %d meters)\n",
		lat, lng, radius)

	// First try the standard Google Places API search
	locations, err := f.googlePlacesSearch(lat, lng, radius)
	if err != nil {
		fmt.Printf("Google Places API search error: %v\n", err)
		// Don't return error yet, try fallback
	}

	// If we found locations, return them
	if len(locations) > 0 {
		fmt.Printf("Found %d Taco Bell locations using Google Places API\n", len(locations))
		return locations, nil
	}

	// Fallback: try text search API which has different matching algorithms
	fmt.Println("No locations found with Nearby Search API, trying Text Search API...")
	locations, err = f.googleTextSearch(lat, lng, radius)
	if err != nil {
		fmt.Printf("Google Text Search API error: %v\n", err)
	}

	// If still no results, try OpenStreetMap as a last resort
	if len(locations) == 0 {
		fmt.Println("No locations found with Google APIs, trying OpenStreetMap...")
		locations, err = f.openStreetMapSearch(lat, lng, radius)
		if err != nil {
			fmt.Printf("OpenStreetMap search error: %v\n", err)
		}
	}

	// For fallbacks, attempt with multiple search terms
	if len(locations) == 0 {
		fmt.Println("Trying fallback search with alternative terms...")
		for _, term := range []string{"Taco Bell", "TacoBell", "taco bell restaurant"} {
			locs, _ := f.googleTextSearch(lat, lng, radius*2, term)
			locations = append(locations, locs...)
		}
	}

	fmt.Printf("Total Taco Bell locations found: %d\n", len(locations))
	return locations, nil
}

// isNearLocation checks if coordinates are within a radius of another location
//func isNearLocation(lat1, lng1, lat2, lng2 float64, radiusKm float64) bool {
//	return haversineDistance(lat1, lng1, lat2, lng2) <= radiusKm
//}

// googlePlacesSearch is the original Google Places API nearby search
func (f *ChilitoBurritoFinder) googlePlacesSearch(lat, lng float64, radius int) ([]TacoBellLocation, error) {
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

		fmt.Printf("Making Places API request: %s\n",
			strings.Replace(requestURL, f.apiKey, "REDACTED", -1))

		// Use the client that might be authenticated
		resp, err := f.client.Get(requestURL)
		if err != nil {
			return nil, err
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		fmt.Printf("Places API response status: %d\n", resp.StatusCode)
		if resp.StatusCode != http.StatusOK {
			fmt.Printf("Response body: %s\n", string(body))
			return nil, fmt.Errorf("places API returned status code %d", resp.StatusCode)
		}

		var result struct {
			Status       string `json:"status"`
			ErrorMessage string `json:"error_message,omitempty"`
			Results      []struct {
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

		if err := json.Unmarshal(body, &result); err != nil {
			return nil, err
		}

		// Check API status
		if result.Status != "OK" && result.Status != "ZERO_RESULTS" {
			if result.ErrorMessage != "" {
				return nil, fmt.Errorf("places API error: %s - %s", result.Status, result.ErrorMessage)
			}
			return nil, fmt.Errorf("places API error: %s", result.Status)
		}

		fmt.Printf("Found %d results in current page\n", len(result.Results))

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
					StoreID:     place.PlaceID, // Use PlaceID as fallback StoreID
				})

				fmt.Printf("Found Taco Bell: %s at %s (%.2f km)\n",
					place.Name, place.Vicinity, distance)
			}
		}

		pagetoken = result.NextPageToken
		if pagetoken == "" {
			break
		}

		// IMPORTANT: Wait between page token requests (required by API)
		fmt.Println("Waiting for next page token to become valid...")
		time.Sleep(2 * time.Second)
	}

	return locations, nil
}

// googleTextSearch uses the Text Search API as an alternative to Nearby Search
func (f *ChilitoBurritoFinder) googleTextSearch(lat, lng float64, radius int, query ...string) ([]TacoBellLocation, error) {
	endpoint := "https://maps.googleapis.com/maps/api/place/textsearch/json"

	// Default search term
	searchTerm := "Taco Bell"
	if len(query) > 0 && query[0] != "" {
		searchTerm = query[0]
	}

	params := url.Values{}
	params.Add("query", searchTerm)
	params.Add("location", fmt.Sprintf("%f,%f", lat, lng))
	params.Add("radius", strconv.Itoa(radius))

	// Only add API key if not using OAuth
	if !f.useOAuth {
		params.Add("key", f.apiKey)
	}

	requestURL := endpoint + "?" + params.Encode()
	fmt.Printf("Making Text Search API request: %s\n",
		strings.Replace(requestURL, f.apiKey, "REDACTED", -1))

	resp, err := f.client.Get(requestURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("text search API returned status code %d", resp.StatusCode)
	}

	var result struct {
		Results []struct {
			PlaceID          string `json:"place_id"`
			Name             string `json:"name"`
			FormattedAddress string `json:"formatted_address"`
			Geometry         struct {
				Location struct {
					Lat float64 `json:"lat"`
					Lng float64 `json:"lng"`
				} `json:"location"`
			} `json:"geometry"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var locations []TacoBellLocation
	for _, place := range result.Results {
		if strings.Contains(strings.ToLower(place.Name), "taco bell") {
			// Calculate distance
			distance := haversineDistance(lat, lng,
				place.Geometry.Location.Lat,
				place.Geometry.Location.Lng)

			// Get additional details
			details, _ := f.getPlaceDetails(place.PlaceID)

			locations = append(locations, TacoBellLocation{
				PlaceID:     place.PlaceID,
				Name:        place.Name,
				Address:     place.FormattedAddress,
				Distance:    distance,
				PhoneNumber: details.PhoneNumber,
			})

			fmt.Printf("Found Taco Bell (text search): %s at %s (%.2f km)\n",
				place.Name, place.FormattedAddress, distance)
		}
	}

	return locations, nil
}

// openStreetMapSearch searches for Taco Bell locations using OSM Overpass API
func (f *ChilitoBurritoFinder) openStreetMapSearch(lat, lng float64, radius int) ([]TacoBellLocation, error) {
	// Convert radius from meters to degrees (approximate)
	radiusDegrees := float64(radius) / 111000.0 // 1 degree is roughly 111 km

	// Build Overpass query to find Taco Bell locations
	bbox := fmt.Sprintf("%.6f,%.6f,%.6f,%.6f",
		lng-radiusDegrees, lat-radiusDegrees,
		lng+radiusDegrees, lat+radiusDegrees)

	query := fmt.Sprintf(`[out:json];
		(
		  node["amenity"="fast_food"]["name"~"Taco Bell",i](%s);
		  way["amenity"="fast_food"]["name"~"Taco Bell",i](%s);
		  relation["amenity"="fast_food"]["name"~"Taco Bell",i](%s);
		);
		out center;`, bbox, bbox, bbox)

	// URL encode the query
	encoded := url.QueryEscape(query)
	requestURL := "https://overpass-api.de/api/interpreter?data=" + encoded

	fmt.Println("Making OpenStreetMap Overpass API request...")

	client := http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(requestURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("overpass API returned status %d", resp.StatusCode)
	}

	var result struct {
		Elements []struct {
			Type string `json:"type"`
			ID   int64  `json:"id"`
			Tags struct {
				Name        string `json:"name"`
				Housenumber string `json:"addr:housenumber"`
				Street      string `json:"addr:street"`
				City        string `json:"addr:city"`
				State       string `json:"addr:state"`
				Postcode    string `json:"addr:postcode"`
				Phone       string `json:"phone"`
			} `json:"tags"`
			Lat    float64 `json:"lat"`
			Lon    float64 `json:"lon"`
			Center struct {
				Lat float64 `json:"lat"`
				Lon float64 `json:"lon"`
			} `json:"center"`
		} `json:"elements"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var locations []TacoBellLocation
	for _, element := range result.Elements {
		// Get coordinates based on element type
		nodeLat, nodeLng := element.Lat, element.Lon
		if element.Type != "node" {
			// For ways and relations, use center
			nodeLat, nodeLng = element.Center.Lat, element.Center.Lon
		}

		// Build address from components
		address := ""
		if element.Tags.Housenumber != "" && element.Tags.Street != "" {
			address = element.Tags.Housenumber + " " + element.Tags.Street
		}
		if element.Tags.City != "" {
			if address != "" {
				address += ", "
			}
			address += element.Tags.City
		}
		if element.Tags.State != "" {
			if address != "" {
				address += ", "
			}
			address += element.Tags.State
		}
		if element.Tags.Postcode != "" {
			if address != "" {
				address += " "
			}
			address += element.Tags.Postcode
		}

		if address == "" {
			address = "Address unknown"
		}

		// Calculate distance
		distance := haversineDistance(lat, lng, nodeLat, nodeLng)

		// Build unique ID for OSM elements
		placeID := fmt.Sprintf("osm-%s-%d", element.Type, element.ID)

		locations = append(locations, TacoBellLocation{
			PlaceID:     placeID,
			Name:        element.Tags.Name,
			Address:     address,
			Distance:    distance,
			PhoneNumber: element.Tags.Phone,
			StoreID:     placeID, // Use the OSM ID as a fallback store ID
		})

		fmt.Printf("Found Taco Bell (OSM): %s at %s (%.2f km)\n",
			element.Tags.Name, address, distance)
	}

	return locations, nil
}

// getPlaceDetails gets additional details for a place
func (f *ChilitoBurritoFinder) getPlaceDetails(placeID string) (PlaceDetails, error) {
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
		return PlaceDetails{}, err
	}
	defer resp.Body.Close()

	var result struct {
		Result struct {
			FormattedPhoneNumber string `json:"formatted_phone_number"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return PlaceDetails{}, err
	}

	return PlaceDetails{
		PhoneNumber: result.Result.FormattedPhoneNumber,
	}, nil
}

// similarAddresses checks if two addresses are similar enough to be considered the same location
func similarAddresses(addr1, addr2 string) bool {
	// Normalize both addresses: lowercase, remove punctuation, standardize whitespace
	normalize := func(s string) string {
		s = strings.ToLower(s)
		s = regexp.MustCompile(`[^\w\s]`).ReplaceAllString(s, " ")
		s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
		s = strings.TrimSpace(s)
		return s
	}

	norm1 := normalize(addr1)
	norm2 := normalize(addr2)

	// Direct match after normalization
	if norm1 == norm2 {
		return true
	}

	// Check if one is contained in the other
	if strings.Contains(norm1, norm2) || strings.Contains(norm2, norm1) {
		return true
	}

	// Split into components and check for partial matches
	parts1 := strings.Fields(norm1)
	parts2 := strings.Fields(norm2)

	// Count matching words
	matches := 0
	for _, p1 := range parts1 {
		if len(p1) <= 2 { // Skip very short words like "a", "an", "of"
			continue
		}
		for _, p2 := range parts2 {
			if p1 == p2 || (len(p1) > 4 && strings.Contains(p2, p1)) || (len(p2) > 4 && strings.Contains(p1, p2)) {
				matches++
				break
			}
		}
	}

	// If we have enough matching words or components, consider it similar
	// The threshold depends on the length of the address
	minMatches := 2
	if len(parts1) > 5 || len(parts2) > 5 {
		minMatches = 3
	}

	return matches >= minMatches
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
		success := false

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
				success = true
				break
			}

			if resp != nil {
				resp.Body.Close()
				resp = nil
			}

			// Wait before retrying
			time.Sleep(time.Duration(attempt+1) * time.Second)
		}

		// If all attempts failed
		if !success {
			if success {
				fmt.Printf("Failed to access %s after multiple attempts: %v\n", menuURL, err)
			} else {
				fmt.Printf("Failed to access %s after multiple attempts\n", menuURL)
			}
			continue
		}

		// At this point, we know resp is not nil and the status is OK
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

// Replace the haversineDistance function with a more accurate implementation
func haversineDistance(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6371 // Earth radius in kilometers

	// Convert latitude and longitude from degrees to radians
	lat1Rad := lat1 * math.Pi / 180
	lng1Rad := lng1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	lng2Rad := lng2 * math.Pi / 180

	// Differences in coordinates
	dLat := lat2Rad - lat1Rad
	dLng := lng2Rad - lng1Rad

	// Haversine formula
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(dLng/2)*math.Sin(dLng/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return R * c
}

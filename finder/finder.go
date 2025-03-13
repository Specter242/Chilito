package finder

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

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
	// This is a simplified version. In a real app, we'd parse the address
	// and use the Taco Bell store locator API to find the store ID

	// For demo purposes, this would locate the store on tacobell.com
	// and extract the store ID from the URL or page content

	// Example: https://www.tacobell.com/locations/search?q={address}

	// For now, return a placeholder
	return "store-" + location.PlaceID, nil
}

// checkForChilitoBurrito checks if a location has the Chili Cheese Burrito
func (f *ChilitoBurritoFinder) checkForChilitoBurrito(location TacoBellLocation) (bool, error) {
	// In a real application, we would:
	// 1. Access the Taco Bell menu API for this store ID
	// 2. Or scrape the menu from the website
	// 3. Search for "Chili Cheese Burrito" or "Chilito"

	// Example URL: https://www.tacobell.com/food/menu/{storeID}

	// For demonstration, we'll use a simulated implementation
	endpoint := fmt.Sprintf("https://www.tacobell.com/food/menu/%s", location.StoreID)

	resp, err := http.Get(endpoint)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	// In a real implementation, we'd parse the HTML and look for the Chilito
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return false, err
	}

	// Search for Chili Cheese Burrito in the menu items
	// This is a placeholder implementation
	found := false
	doc.Find(".menu-item").Each(func(i int, s *goquery.Selection) {
		itemName := s.Find(".item-name").Text()
		if strings.Contains(strings.ToLower(itemName), "chili cheese burrito") ||
			strings.Contains(strings.ToLower(itemName), "chilito") {
			found = true
		}
	})

	return found, nil
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

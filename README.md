# Chilito - Taco Bell Chili Cheese Burrito Finder

This Go application helps you locate the nearest Taco Bell serving the legendary Chili Cheese Burrito (aka Chilito).

## Prerequisites

- Go 1.16 or higher
- Either:
  - Google API key with Places API and Geocoding API enabled, or
  - Google OAuth2 client credentials (more reliable)

## Authentication Setup

### Option 1: API Key (Basic)
1. Go to the [Google Cloud Console](https://console.cloud.google.com/)
2. Navigate to your project and select "APIs & Services" > "Credentials"
3. Find your API key and click "Edit"
4. Under "API restrictions", add both "Places API" and "Geocoding API"

### Option 2: OAuth2 (Recommended)
1. Go to the [Google Cloud Console](https://console.cloud.google.com/)
2. Navigate to your project and select "APIs & Services" > "Credentials"
3. Click "Create Credentials" > "OAuth client ID"
4. Select "Desktop app" for the application type
5. Download the JSON file with client credentials
6. Use the `--oauth` flag when running the application

## Installation

```bash
git clone https://github.com/yourusername/chilito.git
cd chilito
go mod tidy
go build
```

## Usage

### Basic authentication with API key:
```bash
./chilito --address="123 Main St, Anytown, USA"
```

### OAuth2 authentication (recommended):
```bash
./chilito --address="123 Main St, Anytown, USA" --oauth
```

### Options

- `--address`: Your starting location (required)
- `--radius`: Search radius in meters (default: 50000, which is 50km)
- `--verbose`: Enable verbose output for debugging
- `--oauth`: Use OAuth2 authentication instead of API key
- `--credentials`: Path to OAuth client credentials JSON file (defaults to user's Downloads folder)

## How It Works

1. The application geocodes your address to get latitude/longitude
2. It searches for Taco Bell locations near your coordinates
3. For each location (sorted by distance), it checks the menu for the Chili Cheese Burrito
4. When a location serving the burrito is found, it returns the details of that location

## Notes

- The application includes advanced scraping techniques to detect the Chili Cheese Burrito on Taco Bell menus
- It handles address similarity matching to properly identify store locations
- Multiple fallback methods are implemented to maximize the chances of finding the Chilito
- Includes backup geocoding via OpenStreetMap if Google geocoding fails
- The search process may take a few minutes as it needs to check each Taco Bell location's menu

## Troubleshooting

If you encounter geocoding errors:
- Try using the OAuth authentication method with `--oauth` flag
- Try adding more details to your address (street, city, state, zip)
- Use the `--verbose` flag to see more detailed error messages
- For international addresses, include the country name

## Implementation Details

- Uses the Google Places API to find nearby Taco Bell restaurants
- Implements web scraping with goquery to check Taco Bell menus
- The search is conducted in order of proximity, so the nearest location is identified first

## License

MIT

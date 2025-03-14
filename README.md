# Chilito - Taco Bell Chili Cheese Burrito Finder

This Go application helps you locate the nearest Taco Bell serving the legendary Chili Cheese Burrito (aka Chilito).

## Prerequisites

- Go 1.16 or higher
- Google Places API key (already configured in the application)

## Installation

```bash
git clone https://github.com/yourusername/chilito.git
cd chilito
go mod tidy
go build
```

## Usage

```bash
./chilito --address="123 Main St, Anytown, USA"
```

### Options

- `--address`: Your starting location (required)
- `--radius`: Search radius in meters (default: 50000, which is 50km)

## How It Works

1. The application geocodes your address to get latitude/longitude
2. It searches for Taco Bell locations near your coordinates
3. For each location (sorted by distance), it checks the menu for the Chili Cheese Burrito
4. When a location serving the burrito is found, it returns the details of that location

## Notes

- The application includes advanced scraping techniques to detect the Chili Cheese Burrito on Taco Bell menus
- It handles address similarity matching to properly identify store locations
- Multiple fallback methods are implemented to maximize the chances of finding the Chilito
- The search process may take a few minutes as it needs to check each Taco Bell location's menu

## Implementation Details

- Uses the Google Places API to find nearby Taco Bell restaurants
- Implements web scraping with goquery to check Taco Bell menus
- The search is conducted in order of proximity, so the nearest location is identified first

## License

MIT

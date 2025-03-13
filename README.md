# Chilito - Taco Bell Chili Cheese Burrito Finder

This Go application helps you locate the nearest Taco Bell serving the legendary Chili Cheese Burrito (aka Chilito).

## Prerequisites

- Go 1.16 or higher
- Google Places API key

## Installation

```bash
git clone https://github.com/yourusername/chilito.git
cd chilito
go build
```

## Usage

```bash
./chilito --address="123 Main St, Anytown, USA" --api-key="your-google-api-key"
```

### Options

- `--address`: Your starting location (required)
- `--radius`: Search radius in meters (default: 50000, which is 50km)
- `--api-key`: Your Google Places API key (required)

## How It Works

1. The application geocodes your address to get latitude/longitude
2. It searches for Taco Bell locations near your coordinates
3. For each location (sorted by distance), it checks the menu for the Chili Cheese Burrito
4. When a location serving the burrito is found, it returns the details of that location

## Notes

- This application requires a valid Google Places API key with billing enabled
- In a production environment, you would want to implement caching and rate limiting
- The menu checking functionality is a simplified example - a real implementation would need to handle the actual Taco Bell website structure

## License

MIT

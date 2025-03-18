# Chilito Burrito Finder

A command-line utility for finding Taco Bell locations that serve the elusive Chili Cheese Burrito (aka "Chilito").

## About the Chilito

The Chili Cheese Burrito, affectionately known as the "Chilito" by fans, was a staple on the Taco Bell menu in the 1990s. Over time, it was removed from the national menu but continues to be available at select locations. This utility helps you find those locations.

## Features

- Search for Taco Bell locations near a specified address 
- Automatically checks menu items to determine if a location serves the Chili Cheese Burrito
- Uses Taco Bell's website APIs to get accurate location information
- Falls back to alternative geocoding and location search when needed
- Pure Go implementation with no external API keys required

## Installation

### From Source

Clone the repository and build the executable:

```bash
git clone https://github.com/yourusername/chilito.git
cd chilito
go build
```

## Usage

Run the utility with the following command:

```bash
./chilito -address "123 Main St, Anytown, USA" -radius 50000
```

### Options

- `-address`: The address to search from (required)
- `-radius`: Search radius in meters (default: 100,000 meters or about 62 miles)
- `-verbose`: Enable verbose output for debugging
- `-delay`: Add delay between API calls in seconds (for debugging)

### Example

```bash
./chilito -address "Alpharetta, GA 30004" -radius 100000
```

## How It Works

1. The utility first converts your address to geographic coordinates using Taco Bell's geocoding API
2. It then searches for Taco Bell locations within the specified radius of those coordinates
3. For each location found, it checks the menu for the Chilito/Chili Cheese Burrito using web scraping
4. Once a location with the Chilito is found, details are displayed including address and distance

## Contributing

Contributions are welcome! If you know of specific Taco Bell locations that serve the Chili Cheese Burrito, you can add them to the knownChilitoLocations map in the finder.go file.

## License

[MIT License](LICENSE)

package finder

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// ClientConfig contains OAuth2 client credentials
type ClientConfig struct {
	Installed struct {
		ClientID                string   `json:"client_id"`
		ProjectID               string   `json:"project_id"`
		AuthURI                 string   `json:"auth_uri"`
		TokenURI                string   `json:"token_uri"`
		AuthProviderX509CertURL string   `json:"auth_provider_x509_cert_url"`
		ClientSecret            string   `json:"client_secret"`
		RedirectURIs            []string `json:"redirect_uris"`
	} `json:"installed"`
}

// OAuthClient creates an HTTP client with OAuth2 authentication
func GetAuthenticatedClient(credentialsPath string) (*http.Client, error) {
	// Read client secret
	clientConfig, err := readClientConfig(credentialsPath)
	if err != nil {
		return nil, err
	}

	// Configure the OAuth2 config
	config := &oauth2.Config{
		ClientID:     clientConfig.Installed.ClientID,
		ClientSecret: clientConfig.Installed.ClientSecret,
		RedirectURL:  "http://localhost",
		Scopes: []string{
			"https://www.googleapis.com/auth/places",
			"https://www.googleapis.com/auth/geocoding",
		},
		Endpoint: google.Endpoint,
	}

	// Check if we have a token file
	tokenFile := filepath.Join(os.TempDir(), "chilito-token.json")
	token, err := tokenFromFile(tokenFile)
	if err != nil {
		// No token file, we need to get one via web browser flow
		token, err = getTokenFromWeb(config)
		if err != nil {
			return nil, err
		}
		saveToken(tokenFile, token)
	}

	// Create HTTP client with the token
	return config.Client(context.Background(), token), nil
}

// readClientConfig reads the client configuration from a JSON file
func readClientConfig(path string) (*ClientConfig, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("unable to read client secret file: %v", err)
	}

	var config ClientConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("unable to parse client secret file: %v", err)
	}

	return &config, nil
}

// getTokenFromWeb gets a token from the web flow
func getTokenFromWeb(config *oauth2.Config) (*oauth2.Token, error) {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser: \n%v\n", authURL)
	fmt.Println("Enter the authorization code: ")

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		return nil, fmt.Errorf("unable to read authorization code: %v", err)
	}

	token, err := config.Exchange(context.Background(), authCode)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve token from web: %v", err)
	}
	return token, nil
}

// tokenFromFile retrieves a token from a local file
func tokenFromFile(file string) (*oauth2.Token, error) {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var token oauth2.Token
	err = json.Unmarshal(data, &token)
	return &token, err
}

// saveToken saves a token to a file
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	data, err := json.Marshal(token)
	if err != nil {
		fmt.Printf("Unable to save token: %v\n", err)
		return
	}
	err = ioutil.WriteFile(path, data, 0600)
	if err != nil {
		fmt.Printf("Unable to cache oauth token: %v\n", err)
	}
}

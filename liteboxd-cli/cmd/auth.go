package cmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/fslongjin/liteboxd/liteboxd-cli/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/term"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage authentication",
	Long:  `Login, check status, and logout from a LiteBoxd server.`,
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login and store an API key",
	Long: `Login to a LiteBoxd server with username and password.
This will create an API key and store it in your config file for subsequent CLI commands.`,
	Example: `  # Login with interactive prompts
  liteboxd auth login

  # Login to a specific server
  liteboxd auth login --api-server https://api.example.com/api/v1`,
	RunE: runAuthLogin,
}

var authStatusCmd = &cobra.Command{
	Use:     "status",
	Short:   "Show current authentication status",
	Example: `  liteboxd auth status`,
	RunE:    runAuthStatus,
}

var authLogoutCmd = &cobra.Command{
	Use:     "logout",
	Short:   "Clear stored authentication token",
	Example: `  liteboxd auth logout`,
	RunE:    runAuthLogout,
}

func init() {
	rootCmd.AddCommand(authCmd)
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authStatusCmd)
	authCmd.AddCommand(authLogoutCmd)
}

func getBaseURL() string {
	url := viper.GetString("api-server")
	if url == "" {
		url = "http://localhost:8080/api/v1"
	}
	return strings.TrimRight(url, "/")
}

func runAuthLogin(cmd *cobra.Command, args []string) error {
	baseURL := getBaseURL()
	reader := bufio.NewReader(os.Stdin)

	// Prompt for username
	fmt.Print("Username (default: admin): ")
	username, _ := reader.ReadString('\n')
	username = strings.TrimSpace(username)
	if username == "" {
		username = "admin"
	}

	// Prompt for password (hidden input)
	fmt.Print("Password: ")
	passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println() // newline after hidden input
	if err != nil {
		return fmt.Errorf("failed to read password: %w", err)
	}
	password := string(passwordBytes)
	if password == "" {
		return fmt.Errorf("password cannot be empty")
	}

	// Step 1: Login to get session cookie
	loginBody, _ := json.Marshal(map[string]string{
		"username": username,
		"password": password,
	})

	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	loginResp, err := client.Post(baseURL+"/auth/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer loginResp.Body.Close()

	if loginResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(loginResp.Body)
		var errResp map[string]string
		if json.Unmarshal(body, &errResp) == nil {
			if msg, ok := errResp["error"]; ok {
				return fmt.Errorf("login failed: %s", msg)
			}
		}
		return fmt.Errorf("login failed (HTTP %d)", loginResp.StatusCode)
	}

	// Extract session cookies
	cookies := loginResp.Cookies()
	if len(cookies) == 0 {
		return fmt.Errorf("login succeeded but no session cookie received")
	}

	fmt.Printf("Logged in as %s\n", username)

	// Step 2: Create an API key using the session cookie
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}
	keyName := fmt.Sprintf("cli-%s-%d", hostname, time.Now().Unix())

	createBody, _ := json.Marshal(map[string]string{
		"name": keyName,
	})

	createReq, err := http.NewRequest("POST", baseURL+"/auth/api-keys", bytes.NewReader(createBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	createReq.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		createReq.AddCookie(c)
	}

	createResp, err := client.Do(createReq)
	if err != nil {
		return fmt.Errorf("failed to create API key: %w", err)
	}
	defer createResp.Body.Close()

	if createResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(createResp.Body)
		return fmt.Errorf("failed to create API key (HTTP %d): %s", createResp.StatusCode, string(body))
	}

	var keyResp struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Key    string `json:"key"`
		Prefix string `json:"prefix"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&keyResp); err != nil {
		return fmt.Errorf("failed to parse API key response: %w", err)
	}

	// Step 3: Save the API key to config
	if err := config.SaveToken(keyResp.Key); err != nil {
		// Print the key so the user can manually save it
		fmt.Printf("\nAPI Key created but failed to save to config: %v\n", err)
		fmt.Printf("Please manually add this to your config:\n  token: %s\n", keyResp.Key)
		return nil
	}

	// Clean up temporary session (best-effort)
	logoutReq, err := http.NewRequest("POST", baseURL+"/auth/logout", nil)
	if err == nil {
		for _, c := range cookies {
			logoutReq.AddCookie(c)
		}
		resp, err := client.Do(logoutReq)
		if err == nil {
			resp.Body.Close()
		}
	}

	fmt.Printf("API key created and saved to config (%s)\n", config.GetConfigPath())
	fmt.Printf("  Name: %s\n", keyResp.Name)
	fmt.Printf("  Prefix: lbxk_%s...\n", keyResp.Prefix)
	fmt.Println("\nYou can now use other CLI commands (e.g., liteboxd sandbox list).")
	return nil
}

func runAuthStatus(cmd *cobra.Command, args []string) error {
	baseURL := getBaseURL()
	token := viper.GetString("token")

	if token == "" {
		fmt.Println("Not authenticated. Run 'liteboxd auth login' to authenticate.")
		return nil
	}

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", baseURL+"/auth/me", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		fmt.Println("Token is invalid or expired. Run 'liteboxd auth login' to re-authenticate.")
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected response (HTTP %d)", resp.StatusCode)
	}

	var meResp struct {
		AuthMethod string `json:"auth_method"`
		Username   string `json:"username"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&meResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	fmt.Printf("Authenticated via %s\n", meResp.AuthMethod)
	if meResp.Username != "" {
		fmt.Printf("  Username: %s\n", meResp.Username)
	}
	fmt.Printf("  Server: %s\n", baseURL)
	return nil
}

func runAuthLogout(cmd *cobra.Command, args []string) error {
	token := viper.GetString("token")
	if token == "" {
		fmt.Println("No stored token found. Already logged out.")
		return nil
	}

	if err := config.ClearToken(); err != nil {
		return fmt.Errorf("failed to clear token: %w", err)
	}

	fmt.Println("Token cleared. You are now logged out.")
	return nil
}

package collector

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// authenticate authenticates with Proxmox API
func (c *ProxmoxCollector) authenticate() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Use token authentication if available
	if c.config.TokenID != "" && c.config.TokenSecret != "" {
		return nil // Token auth doesn't need ticket
	}

	// Use password authentication
	apiURL := fmt.Sprintf("https://%s:%d/api2/json/access/ticket", c.config.Host, c.config.Port)

	data := url.Values{}
	data.Set("username", c.config.User)
	data.Set("password", c.config.Password)

	resp, err := c.client.PostForm(apiURL, data)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("authentication failed with status: %d", resp.StatusCode)
	}

	var result struct {
		Data struct {
			Ticket string `json:"ticket"`
			CSRF   string `json:"CSRFPreventionToken"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode auth response: %w", err)
	}

	c.ticket = result.Data.Ticket
	c.csrf = result.Data.CSRF

	return nil
}

// apiRequest makes an authenticated API request
func (c *ProxmoxCollector) apiRequest(path string) ([]byte, error) {
	apiURL := fmt.Sprintf("https://%s:%d/api2/json%s", c.config.Host, c.config.Port, path)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	// Add authentication
	c.mutex.RLock()
	if c.config.TokenID != "" && c.config.TokenSecret != "" {
		req.Header.Set("Authorization", fmt.Sprintf("PVEAPIToken=%s=%s", c.config.TokenID, c.config.TokenSecret))
	} else {
		req.Header.Set("Cookie", fmt.Sprintf("PVEAuthCookie=%s", c.ticket))
		req.Header.Set("CSRFPreventionToken", c.csrf)
	}
	c.mutex.RUnlock()

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

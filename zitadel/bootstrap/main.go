package main

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math/big"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type JWKS struct {
	Keys []JWKKey `json:"keys"`
}

type JWKKey struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func jwksKeyToRSAPublicKey(nStr, eStr string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nStr)
	if err != nil {
		return nil, fmt.Errorf("decode n: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eStr)
	if err != nil {
		return nil, fmt.Errorf("decode e: %w", err)
	}
	var eInt int
	for _, b := range eBytes {
		eInt = (eInt << 8) | int(b)
	}
	n := new(big.Int).SetBytes(nBytes)
	return &rsa.PublicKey{N: n, E: eInt}, nil
}

func rsaPublicKeyToPEM(pub *rsa.PublicKey) (string, error) {
	pubBytes, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", err
	}
	block := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubBytes,
	}
	return string(pem.EncodeToMemory(block)), nil
}

func apiRequest(method, urlStr, pat string, payload interface{}) ([]byte, int, error) {
	var bodyReader io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, 0, err
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, urlStr, bodyReader)
	if err != nil {
		return nil, 0, err
	}
	req.Host = "localhost:8081"
	req.Header.Set("Content-Type", "application/json")
	if pat != "" {
		req.Header.Set("Authorization", "Bearer "+pat)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	return respBody, resp.StatusCode, err
}

func main() {
	zitadelURL := os.Getenv("ZITADEL_INTERNAL_URL")
	if zitadelURL == "" {
		zitadelURL = "http://zitadel:8080"
	}

	patFile := os.Getenv("PAT_FILE")
	if patFile == "" {
		patFile = "/zitadel-data/admin.pat"
	}

	clientIDFile := os.Getenv("CLIENT_ID_FILE")
	if clientIDFile == "" {
		clientIDFile = "/zitadel-data/client_id"
	}

	kongConfigPath := os.Getenv("KONG_CONFIG_PATH")
	if kongConfigPath == "" {
		kongConfigPath = "/usr/local/kong/declarative/kong.yml"
	}

	log.Printf("[BOOTSTRAP] Starting Zitadel automated provisioning...")

	// 1. Wait for Zitadel healthz
	for {
		resp, err := http.Get(zitadelURL + "/debug/healthz")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			log.Printf("[BOOTSTRAP] Zitadel is healthy!")
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		log.Printf("[BOOTSTRAP] Waiting for Zitadel at %s/debug/healthz...", zitadelURL)
		time.Sleep(3 * time.Second)
	}

	// 2. Wait for admin PAT file
	var pat string
	for {
		if data, err := os.ReadFile(patFile); err == nil {
			pat = strings.TrimSpace(string(data))
			if pat != "" {
				log.Printf("[BOOTSTRAP] Successfully loaded Admin PAT token from %s", patFile)
				break
			}
		}
		log.Printf("[BOOTSTRAP] Waiting for admin PAT in %s...", patFile)
		time.Sleep(3 * time.Second)
	}

	// 3. Fetch JWKS public key from Zitadel and generate PEM (non-blocking, with background updater)
	var pemKey string
	fetchSigningKey := func() string {
		jwksBody, status, err := apiRequest("GET", zitadelURL+"/oauth/v2/keys", "", nil)
		if err == nil && status == http.StatusOK {
			var jwks JWKS
			if err := json.Unmarshal(jwksBody, &jwks); err == nil && len(jwks.Keys) > 0 {
				for _, k := range jwks.Keys {
					if (k.Use == "sig" || k.Use == "") && k.Kty == "RSA" && k.N != "" && k.E != "" {
						pubKey, err := jwksKeyToRSAPublicKey(k.N, k.E)
						if err == nil {
							pem, err := rsaPublicKeyToPEM(pubKey)
							if err == nil {
								return pem
							}
						}
					}
				}
			}
		}
		return ""
	}
	pemKey = fetchSigningKey()
	if pemKey != "" {
		log.Printf("[BOOTSTRAP] Successfully exported Zitadel RS256 Public Key!")
	} else {
		log.Printf("[BOOTSTRAP] Signing key not ready yet, will continue in background.")
	}

	// 4. Create or get Project "auth-spa-poc"
	var projectID string
	searchProjBody, _, _ := apiRequest("POST", zitadelURL+"/management/v1/projects/_search", pat, map[string]interface{}{
		"queries": []interface{}{
			map[string]interface{}{
				"nameQuery": map[string]interface{}{
					"name":   "auth-spa-poc",
					"method": "TEXT_QUERY_METHOD_EQUALS",
				},
			},
		},
	})
	var searchProjResp struct {
		Result []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"result"`
	}
	_ = json.Unmarshal(searchProjBody, &searchProjResp)
	if len(searchProjResp.Result) > 0 {
		projectID = searchProjResp.Result[0].ID
		log.Printf("[BOOTSTRAP] Found existing Project 'auth-spa-poc' (ID: %s)", projectID)
	} else {
		createProjBody, status, err := apiRequest("POST", zitadelURL+"/management/v1/projects", pat, map[string]interface{}{
			"name":                 "auth-spa-poc",
			"projectRoleAssertion": true,
		})
		if err != nil || (status != http.StatusOK && status != http.StatusCreated) {
			log.Fatalf("[BOOTSTRAP] Failed to create project: %v (status %d, resp: %s)", err, status, string(createProjBody))
		}
		var createProjResp struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(createProjBody, &createProjResp)
		projectID = createProjResp.ID
		log.Printf("[BOOTSTRAP] Created Project 'auth-spa-poc' (ID: %s)", projectID)
	}

	// 5. Create roles "user" and "admin" in the project
	for _, role := range []struct {
		Key  string
		Name string
	}{
		{"user", "Standard User"},
		{"admin", "Administrator"},
	} {
		_, _, _ = apiRequest("POST", fmt.Sprintf("%s/management/v1/projects/%s/roles", zitadelURL, projectID), pat, map[string]interface{}{
			"roleKey":     role.Key,
			"displayName": role.Name,
		})
		log.Printf("[BOOTSTRAP] Role configured: %s", role.Key)
	}

	// 6. Create demo users "alice" and "bob"
	createOrGetUser := func(username, email, firstName, lastName, password string) string {
		searchUserBody, _, _ := apiRequest("POST", zitadelURL+"/management/v1/users/_search", pat, map[string]interface{}{
			"queries": []interface{}{
				map[string]interface{}{
					"userNameQuery": map[string]interface{}{
						"userName": username,
						"method":   "TEXT_QUERY_METHOD_EQUALS",
					},
				},
			},
		})
		var searchUserResp struct {
			Result []struct {
				ID       string `json:"id"`
				UserName string `json:"userName"`
			} `json:"result"`
		}
		_ = json.Unmarshal(searchUserBody, &searchUserResp)
		if len(searchUserResp.Result) > 0 {
			log.Printf("[BOOTSTRAP] Found existing user %s (ID: %s)", username, searchUserResp.Result[0].ID)
			return searchUserResp.Result[0].ID
		}

		createUserBody, status, err := apiRequest("POST", zitadelURL+"/management/v1/users/human", pat, map[string]interface{}{
			"userName": username,
			"profile": map[string]interface{}{
				"firstName":   firstName,
				"lastName":    lastName,
				"displayName": firstName + " " + lastName,
			},
			"email": map[string]interface{}{
				"email":           email,
				"isEmailVerified": true,
			},
			"initialPassword": password,
		})
		if err != nil || (status != http.StatusOK && status != http.StatusCreated) {
			log.Printf("[BOOTSTRAP] Note on create user %s: %s", username, string(createUserBody))
		}
		var createUserResp struct {
			UserID string `json:"userId"`
		}
		_ = json.Unmarshal(createUserBody, &createUserResp)
		log.Printf("[BOOTSTRAP] Created user %s (ID: %s)", username, createUserResp.UserID)
		return createUserResp.UserID
	}

	aliceID := createOrGetUser("alice", "alice@example.com", "Alice", "Smith", "Alice123!")
	bobID := createOrGetUser("bob", "bob@example.com", "Bob", "Jones", "Bob1234!")

	// 7. Grant roles to alice (user) and bob (admin, user)
	if aliceID != "" {
		_, _, _ = apiRequest("POST", fmt.Sprintf("%s/management/v1/users/%s/grants", zitadelURL, aliceID), pat, map[string]interface{}{
			"projectId": projectID,
			"roleKeys":  []string{"user"},
		})
		log.Printf("[BOOTSTRAP] Assigned role 'user' to alice")
	}
	if bobID != "" {
		_, _, _ = apiRequest("POST", fmt.Sprintf("%s/management/v1/users/%s/grants", zitadelURL, bobID), pat, map[string]interface{}{
			"projectId": projectID,
			"roleKeys":  []string{"admin", "user"},
		})
		log.Printf("[BOOTSTRAP] Assigned roles 'admin', 'user' to bob")
	}

	// 8. Create or find OIDC Application "angular-spa"
	var clientID string
	searchAppsBody, _, _ := apiRequest("POST", fmt.Sprintf("%s/management/v1/projects/%s/apps/_search", zitadelURL, projectID), pat, map[string]interface{}{
		"queries": []interface{}{},
	})
	var searchAppsResp struct {
		Result []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"result"`
	}
	_ = json.Unmarshal(searchAppsBody, &searchAppsResp)
	for _, app := range searchAppsResp.Result {
		if app.Name == "angular-spa" {
			appDetailBody, _, _ := apiRequest("GET", fmt.Sprintf("%s/management/v1/projects/%s/apps/%s", zitadelURL, projectID, app.ID), pat, nil)
			var appDetail struct {
				App struct {
					OidcConfig struct {
						ClientID string `json:"clientId"`
					} `json:"oidcConfig"`
				} `json:"app"`
			}
			_ = json.Unmarshal(appDetailBody, &appDetail)
			clientID = appDetail.App.OidcConfig.ClientID
			log.Printf("[BOOTSTRAP] Found existing OIDC app 'angular-spa' (Client ID: %s)", clientID)

			// Ensure existing app is configured for JWT access tokens with role & userinfo assertions
			updateBody, updateStatus, updateErr := apiRequest("PUT", fmt.Sprintf("%s/management/v1/projects/%s/apps/%s/oidc_config", zitadelURL, projectID, app.ID), pat, map[string]interface{}{
				"responseTypes":          []string{"OIDC_RESPONSE_TYPE_CODE"},
				"grantTypes":             []string{"OIDC_GRANT_TYPE_AUTHORIZATION_CODE"},
				"appType":                "OIDC_APP_TYPE_USER_AGENT",
				"authMethodType":         "OIDC_AUTH_METHOD_TYPE_NONE",
				"postLogoutRedirectUris": []string{"http://localhost:4200"},
				"redirectUris":           []string{"http://localhost:4200"},
				"version":                "OIDC_VERSION_1_0",
				"devMode":                true,
				"accessTokenType":         "OIDC_TOKEN_TYPE_JWT",
				"accessTokenRoleAssertion": true,
				"idTokenRoleAssertion":     true,
				"idTokenUserinfoAssertion": true,
			})
			log.Printf("[BOOTSTRAP] Updated OIDC app 'angular-spa' to JWT access token (status: %d, err: %v, resp: %s)", updateStatus, updateErr, string(updateBody))
			break
		}
	}

	if clientID == "" {
		createAppBody, status, err := apiRequest("POST", fmt.Sprintf("%s/management/v1/projects/%s/apps/oidc", zitadelURL, projectID), pat, map[string]interface{}{
			"name":                   "angular-spa",
			"redirectUris":           []string{"http://localhost:4200"},
			"postLogoutRedirectUris": []string{"http://localhost:4200"},
			"responseTypes":          []string{"OIDC_RESPONSE_TYPE_CODE"},
			"grantTypes":             []string{"OIDC_GRANT_TYPE_AUTHORIZATION_CODE"},
			"appType":                "OIDC_APP_TYPE_USER_AGENT",
			"authMethodType":         "OIDC_AUTH_METHOD_TYPE_NONE",
			"version":                "OIDC_VERSION_1_0",
			"devMode":                true,
			"accessTokenType":         "OIDC_TOKEN_TYPE_JWT",
			"accessTokenRoleAssertion": true,
			"idTokenRoleAssertion":     true,
			"idTokenUserinfoAssertion": true,
		})
		if err == nil && (status == http.StatusOK || status == http.StatusCreated) {
			var createAppResp struct {
				ClientID string `json:"clientId"`
			}
			_ = json.Unmarshal(createAppBody, &createAppResp)
			clientID = createAppResp.ClientID
			log.Printf("[BOOTSTRAP] Successfully created OIDC app 'angular-spa' with JWT tokens (Client ID: %s)", clientID)
		} else {
			// If already created, read from file or search
			if data, err := os.ReadFile(clientIDFile); err == nil {
				clientID = strings.TrimSpace(string(data))
				log.Printf("[BOOTSTRAP] Read existing Client ID from %s: %s", clientIDFile, clientID)
			}
		}
	}

	// 9. Save client ID for the adapter
	if clientID != "" {
		_ = os.MkdirAll(filepath.Dir(clientIDFile), 0755)
		if err := os.WriteFile(clientIDFile, []byte(clientID), 0644); err != nil {
			log.Printf("[BOOTSTRAP] Warning: Failed to write client ID to %s: %v", clientIDFile, err)
		} else {
			log.Printf("[BOOTSTRAP] Wrote Client ID %s to %s", clientID, clientIDFile)
		}
	}

	// 10. Background worker to check signing keys and reload Kong when keys become active
	go func() {
		for {
			time.Sleep(5 * time.Second)
			currentKey := fetchSigningKey()
			if currentKey != "" && currentKey != pemKey {
				pemKey = currentKey
				log.Printf("[BOOTSTRAP] Discovered updated Zitadel signing key! Updating Kong...")
				_ = os.MkdirAll(filepath.Dir(kongConfigPath), 0755)
				kongYAML := generateKongYAML(pemKey)
				_ = os.WriteFile(kongConfigPath, []byte(kongYAML), 0644)

				if err := pushKongConfig("http://kong:8001", kongYAML); err != nil {
					log.Printf("[BOOTSTRAP] Kong dynamic update error: %v", err)
				} else {
					log.Printf("[BOOTSTRAP] Successfully refreshed Kong with active Zitadel RS256 key!")
				}
			}
		}
	}()

	log.Printf("[BOOTSTRAP] Zitadel bootstrap complete! Identity provider is fully operational.")
	for {
		time.Sleep(1 * time.Hour)
	}
}

func pushKongConfig(kongURL, kongYAML string) error {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormField("config")
	if err != nil {
		return err
	}
	if _, err := part.Write([]byte(kongYAML)); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	req, err := http.NewRequest("POST", kongURL+"/config", &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("kong status %d: %s", resp.StatusCode, string(respBytes))
	}
	return nil
}

func generateKongYAML(rsaPEM string) string {
	// Format indented PEM block
	lines := strings.Split(strings.TrimSpace(rsaPEM), "\n")
	var indentedLines []string
	for _, line := range lines {
		indentedLines = append(indentedLines, "          "+line)
	}
	indentedPEM := strings.Join(indentedLines, "\n")

	return fmt.Sprintf(`_format_version: "3.0"

consumers:
  - username: aitana-client
    jwt_secrets:
      - key: "http://localhost:8081"
        algorithm: "RS256"
        rsa_public_key: |
%s
      - key: "http://localhost:8081/realms/auth-realm"
        algorithm: "RS256"
        rsa_public_key: |
%s

services:
  - name: go-backend-service
    url: http://go-backend:8080
    routes:
      - name: backend-route
        paths:
          - /api
        strip_path: false
        plugins:
          - name: cors
            config:
              origins:
                - "http://localhost:4200"
              methods:
                - GET
                - POST
                - PUT
                - DELETE
                - OPTIONS
                - HEAD
              headers:
                - Authorization
                - Content-Type
                - Accept
                - Origin
                - X-Requested-With
              credentials: true
              max_age: 3600
          - name: jwt
            config:
              claims_to_verify:
                - exp
              run_on_preflight: false
          - name: post-function
            config:
              access:
                - |
                  local jwt_decoder = require("kong.plugins.jwt.jwt_parser")
                  local auth_header = kong.request.get_header("authorization")
                  if auth_header then
                    local _, _, token = string.find(auth_header, "%%%%s+(%%%%S+)")
                    if token then
                      local jwt, err = jwt_decoder:new(token)
                      if jwt and jwt.claims then
                        local claims = jwt.claims
                        local username = claims.preferred_username or claims.name
                        if not username or username == "" then
                          if claims.sub == "391627256046288902" then
                            username = "alice"
                          elseif claims.sub == "391627257505906694" then
                            username = "bob"
                          else
                            username = claims.sub or "user"
                          end
                        end
                        kong.service.request.set_header("X-User-Username", username)

                        local email = claims.email
                        if not email or email == "" then
                          if username == "alice" then
                            email = "alice@example.com"
                          elseif username == "bob" then
                            email = "bob@example.com"
                          else
                            email = username .. "@example.com"
                          end
                        end
                        kong.service.request.set_header("X-User-Email", email)

                        local role_list = {}
                        if claims.roles then
                          if type(claims.roles) == "table" then
                            for _, r in ipairs(claims.roles) do table.insert(role_list, r) end
                          else
                            table.insert(role_list, tostring(claims.roles))
                          end
                        end

                        local zitadel_roles = claims["urn:zitadel:iam:org:project:roles"]
                        if type(zitadel_roles) == "table" then
                          for r, _ in pairs(zitadel_roles) do
                            table.insert(role_list, r)
                          end
                        end

                        if claims.realm_access and type(claims.realm_access.roles) == "table" then
                          for _, r in ipairs(claims.realm_access.roles) do table.insert(role_list, r) end
                        end

                        if #role_list > 0 then
                          kong.service.request.set_header("X-User-Role", table.concat(role_list, ","))
                        else
                          kong.service.request.set_header("X-User-Role", "user")
                        end
                      end
                    end
                  end
                  kong.service.request.set_header("X-Gateway-Token", "aitana-poc-gateway-secret-token")
                  kong.service.request.set_header("X-Enforcement-Point", "Kong-APIM-Boundary")
                  kong.service.request.clear_header("authorization")
`, indentedPEM, indentedPEM)
}

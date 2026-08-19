package provisioner

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/luaxlou/glow-ops/internal/configmanager"
	"github.com/luaxlou/glow-ops/pkg/api"
	"github.com/redis/go-redis/v9"
)

// RedisConfig stores the Redis configuration
type RedisConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Password string `json:"password"`
	UpdatedAt string `json:"updated_at"`
}

// RedisUserConfig stores specific user credentials for a redis user
type RedisUserConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// ProvisionRedisRequest represents the request to provision Redis
type ProvisionRedisRequest struct {
	Mode             string `json:"mode"` // "create_or_use"
	ExistingPassword string `json:"existingPassword,omitempty"` // For accessing existing users
	DryRun           bool   `json:"dryRun,omitempty"` // If true, only plan changes without executing
}

// ProvisionRedisResult represents the detailed result of Redis provisioning
type ProvisionRedisResult struct {
	Action     string `json:"action"` // "created", "reused", "credentials_stored"
	Username   string `json:"username"`
	Addr       string `json:"addr"`
	ChangeType string `json:"changeType"` // "create", "update", "no_change"
	Message    string `json:"message"`
}

// ProvisionRedis provisions Redis access for an app
func ProvisionRedis(appName string, req ProvisionRedisRequest) api.Response {
	// 1. Load System Redis Config (Admin)
	var adminConfig RedisConfig
	if err := configmanager.GetSystemConfigJSON("redis_info", &adminConfig); err != nil {
		return api.Response{Success: false, Message: "Redis system config not found. Run 'nova-server add redis' first."}
	}
	if adminConfig.Host == "" {
		return api.Response{Success: false, Message: "Redis not configured."}
	}

	// 2. Connect as Admin
	rdb := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", adminConfig.Host, adminConfig.Port),
		Password: adminConfig.Password,
		DB:       0,
	})
	defer rdb.Close()

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		return api.Response{Success: false, Message: fmt.Sprintf("Failed to connect to Redis admin: %v", err)}
	}

	// Use appName as username for simplicity
	username := appName
	var userPassword string
	result := ProvisionRedisResult{
		Username: username,
		Addr:     fmt.Sprintf("%s:%d", adminConfig.Host, adminConfig.Port),
	}

	// Check if user exists
	_, err := rdb.Do(context.Background(), "ACL", "GETUSER", username).Result()
	exists := err == nil
	if err == redis.Nil {
		exists = false
	}

	if exists {
		// Scenario: User Exists
		log.Printf("App %s requested existing Redis User %s", appName, username)

		var redisUsers map[string]RedisUserConfig
		configmanager.GetSystemConfigJSON("redis_users", &redisUsers)
		if redisUsers == nil {
			redisUsers = make(map[string]RedisUserConfig)
		}

		if creds, ok := redisUsers[username]; ok {
			log.Printf("Found stored credentials for Redis User %s", username)
			userPassword = creds.Password
			result.Action = "reused"
			result.ChangeType = "no_change"
			result.Message = fmt.Sprintf("Reusing existing Redis user '%s'", username)
		} else {
			// Need user to provide credentials
			log.Printf("No stored credentials for Redis User %s. Client needs to provide credentials.", username)

			if req.ExistingPassword == "" {
				// Return structured error indicating credentials are needed
				return api.Response{
					Success: false,
					Message: fmt.Sprintf("Redis user '%s' already exists but credentials are not stored. Please provide the existing password.", username),
					Data: map[string]any{
						"error_code": "needs_credentials",
						"username":   username,
						"hint":       "Provide the Redis user password in the 'existingPassword' field",
					},
				}
			}

			if !req.DryRun {
				// Verify credentials provided by client
				userPassword = req.ExistingPassword

				checkClient := redis.NewClient(&redis.Options{
					Addr:     fmt.Sprintf("%s:%d", adminConfig.Host, adminConfig.Port),
					Username: username,
					Password: userPassword,
				})
				if err := checkClient.Ping(context.Background()).Err(); err != nil {
					checkClient.Close()
					return api.Response{Success: false, Message: fmt.Sprintf("Verification failed: %v", err)}
				}
				checkClient.Close()

				// Save
				redisUsers[username] = RedisUserConfig{Username: username, Password: userPassword}
				if err := configmanager.SetSystemConfigJSON("redis_users", redisUsers); err != nil {
					log.Printf("Warning: Failed to save redis_users: %v", err)
				}
			}

			result.Action = "credentials_stored"
			result.ChangeType = "update"
			result.Message = fmt.Sprintf("Stored credentials for existing Redis user '%s'", username)
		}

	} else {
		// Scenario: User Does Not Exist -> Create
		log.Printf("App %s requested NEW Redis User %s. Creating...", appName, username)

		if req.DryRun {
			// Dry run - just return what would happen
			result.Action = "created"
			result.ChangeType = "create"
			result.Message = fmt.Sprintf("Would create new Redis user '%s'", username)

			return api.Response{
				Success: true,
				Message: "Dry run: Redis user would be created",
				Data: map[string]any{
					"redis": map[string]any{
						"addr":     result.Addr,
						"username": username,
						"wouldCreate": true,
					},
					"result": result,
				},
			}
		}

		userPassword = generateRandomPassword(32)

		// ACL SETUSER <name> on >password ~* +@all
		err := rdb.Do(context.Background(), "ACL", "SETUSER", username, "on", ">"+userPassword, "~*", "+@all").Err()

		if err != nil {
			if strings.Contains(err.Error(), "unknown command") {
				return api.Response{Success: false, Message: fmt.Sprintf("Failed to create Redis user (ACL required): %v", err)}
			}
			return api.Response{Success: false, Message: fmt.Sprintf("Failed to create Redis user: %v", err)}
		}

		// Save
		var redisUsers map[string]RedisUserConfig
		configmanager.GetSystemConfigJSON("redis_users", &redisUsers)
		if redisUsers == nil {
			redisUsers = make(map[string]RedisUserConfig)
		}
		redisUsers[username] = RedisUserConfig{Username: username, Password: userPassword}
		if err := configmanager.SetSystemConfigJSON("redis_users", redisUsers); err != nil {
			log.Printf("Warning: Failed to save redis_users: %v", err)
		}

		result.Action = "created"
		result.ChangeType = "create"
		result.Message = fmt.Sprintf("Created new Redis user '%s'", username)
	}

	// Update App Config (skip in dry run)
	if !req.DryRun {
		appConfig := map[string]any{
			"redis": map[string]any{
				"addr":     fmt.Sprintf("%s:%d", adminConfig.Host, adminConfig.Port),
				"username": username,
				"password": userPassword,
				"db":       0, // Default DB
			},
		}

		if err := configmanager.Set(appName, appConfig, true); err != nil {
			log.Printf("Warning: Failed to update app config for %s: %v", appName, err)
		}

		// Return with detailed result
		return api.Response{
			Success: true,
			Data: map[string]any{
				"redis": map[string]any{
					"addr":     result.Addr,
					"username": username,
					"password": userPassword,
					"db":       0,
				},
				"result": result,
			},
			Message: result.Message,
		}
	}

	// Dry run response
	return api.Response{
		Success: true,
		Data: map[string]any{
			"redis": map[string]any{
				"addr":     result.Addr,
				"username": username,
				"password": "***",
				"db":       0,
			},
			"result": result,
		},
		Message: "Dry run: " + result.Message,
	}
}

